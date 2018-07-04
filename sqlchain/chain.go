/*
 * Copyright 2018 The ThunderDB Authors.
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

package sqlchain

import (
	"bytes"
	"encoding/binary"
	"sync"
	"time"

	bolt "github.com/coreos/bbolt"
	log "github.com/sirupsen/logrus"
	"gitlab.com/thunderdb/ThunderDB/crypto/hash"
	"gitlab.com/thunderdb/ThunderDB/crypto/kms"
	ct "gitlab.com/thunderdb/ThunderDB/sqlchain/types"
	"gitlab.com/thunderdb/ThunderDB/utils"
	wt "gitlab.com/thunderdb/ThunderDB/worker/types"
)

var (
	metaBucket              = [4]byte{0x0, 0x0, 0x0, 0x0}
	metaStateKey            = []byte("thunderdb-state")
	metaBlockIndexBucket    = []byte("thunderdb-block-index-bucket")
	metaHeightIndexBucket   = []byte("thunderdb-query-height-index-bucket")
	metaRequestIndexBucket  = []byte("thunderdb-query-reqeust-index-bucket")
	metaResponseIndexBucket = []byte("thunderdb-query-response-index-bucket")
	metaAckIndexBucket      = []byte("thunderdb-query-ack-index-bucket")
)

// HeightToKey converts a height in int32 to a key in bytes.
func HeightToKey(h int32) (key []byte) {
	key = make([]byte, 4)
	binary.BigEndian.PutUint32(key, uint32(h))
	return
}

// KeyToHeight converts a height back from a key in bytes.
func KeyToHeight(k []byte) int32 {
	return int32(binary.BigEndian.Uint32(k))
}

// State represents a snapshot of current best chain.
type State struct {
	node   *blockNode
	Head   hash.Hash
	Height int32
}

// Runtime represents a chain runtime state.
type Runtime struct {
	sync.RWMutex // Protects following fields.

	// Offset is the time difference calculated by: coodinatedChainTime - time.Now().
	//
	// TODO(leventeliu): update Offset in ping cycle.
	Offset time.Duration

	// Period is the block producing cycle.
	Period time.Duration

	// TimeResolution is the maximum time frame between each cycle.
	TimeResolution time.Duration

	// ChainInitTime is the initial cycle time, when the Genesis blcok is produced.
	ChainInitTime time.Time

	// NextTurn is the height of the next block.
	NextTurn int32

	pendingBlock *ct.Block
	stopCh       chan struct{}
}

// UpdateTime updates the current coodinated chain time.
func (r *Runtime) UpdateTime(now time.Time) {
	r.Lock()
	defer r.Unlock()
	r.Offset = now.Sub(time.Now())
}

// Now returns the current coodinated chain time.
func (r *Runtime) Now() time.Time {
	r.RLock()
	defer r.RUnlock()
	return time.Now().Add(r.Offset)
}

// SetNextTurn prepares the runtime state for the next turn.
func (r *Runtime) SetNextTurn() {
	r.Lock()
	defer r.Unlock()
	r.NextTurn++
}

// Stop sends a signal to the Runtime stop channel by closing it.
func (r *Runtime) Stop() {
	close(r.stopCh)
}

// TillNextTurn returns the current time reading and the duration till the next turn. If duration
// is less or equal to 0, use the time reading to run the next cycle - this avoids some problem
// caused by concurrently time synchronization.
func (r *Runtime) TillNextTurn() (t time.Time, d time.Duration) {
	t = r.Now()
	d = r.ChainInitTime.Add(time.Duration(r.NextTurn) * r.Period).Sub(t)

	if d > r.TimeResolution {
		d = r.TimeResolution
	}

	return
}

// MarshalBinary implements BinaryMarshaler.
func (s *State) MarshalBinary() ([]byte, error) {
	buffer := bytes.NewBuffer(nil)

	if err := utils.WriteElements(buffer, binary.BigEndian,
		s.Head,
		s.Height,
	); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}

// UnmarshalBinary implements BinaryUnmarshaler.
func (s *State) UnmarshalBinary(b []byte) (err error) {
	reader := bytes.NewReader(b)
	return utils.ReadElements(reader, binary.BigEndian,
		&s.Head,
		&s.Height,
	)
}

// Chain represents a sql-chain.
type Chain struct {
	cfg   *Config
	db    *bolt.DB
	bi    *blockIndex
	qi    *QueryIndex
	rt    *Runtime
	state *State

	// Only for test
	isMyTurn bool
}

// NewChain creates a new sql-chain struct.
func NewChain(cfg *Config) (chain *Chain, err error) {
	err = cfg.Genesis.VerifyAsGenesis()

	if err != nil {
		return
	}

	// Open DB file
	db, err := bolt.Open(cfg.DataDir, 0600, nil)

	if err != nil {
		return
	}

	// Create buckets for chain meta
	if err = db.Update(func(tx *bolt.Tx) (err error) {
		bucket, err := tx.CreateBucketIfNotExists(metaBucket[:])

		if err != nil {
			return
		}

		if _, err = bucket.CreateBucketIfNotExists(metaBlockIndexBucket); err != nil {
			return
		}

		_, err = bucket.CreateBucketIfNotExists(metaHeightIndexBucket)
		return
	}); err != nil {
		return
	}

	// Create chain state
	pub, err := kms.GetLocalPublicKey()

	if err != nil {
		return
	}

	chain = &Chain{
		cfg: cfg,
		db:  db,
		bi:  newBlockIndex(cfg),
		qi:  NewQueryIndex(),
		rt: &Runtime{
			Offset:         time.Duration(0),
			Period:         cfg.Period,
			ChainInitTime:  cfg.Genesis.SignedHeader.Timestamp,
			TimeResolution: cfg.TimeResolution,
			NextTurn:       1,
			pendingBlock: &ct.Block{
				SignedHeader: ct.SignedHeader{
					Header: ct.Header{
						Version:     0x01000000,
						Producer:    cfg.Server.ID,
						GenesisHash: cfg.Genesis.SignedHeader.BlockHash,
						ParentHash:  cfg.Genesis.SignedHeader.BlockHash,
					},
					Signee: pub,
				},
			},
			stopCh: make(chan struct{}),
		},
		state: &State{
			node:   nil,
			Head:   cfg.Genesis.SignedHeader.GenesisHash,
			Height: -1,
		},
	}

	err = chain.PushBlock(cfg.Genesis)

	if err != nil {
		return nil, err
	}

	return
}

// LoadChain loads the chain state from the specified database and rebuilds a memory index.
func LoadChain(cfg *Config) (chain *Chain, err error) {
	// Open DB file
	db, err := bolt.Open(cfg.DataDir, 0600, nil)

	if err != nil {
		return
	}

	// Create chain state
	pub, err := kms.GetLocalPublicKey()

	if err != nil {
		return
	}

	chain = &Chain{
		cfg: cfg,
		db:  db,
		bi:  newBlockIndex(cfg),
		qi:  NewQueryIndex(),
		rt: &Runtime{
			Offset:         time.Duration(0),
			Period:         cfg.Period,
			TimeResolution: cfg.TimeResolution,
			NextTurn:       1,
			pendingBlock: &ct.Block{
				SignedHeader: ct.SignedHeader{
					Header: ct.Header{
						Version:     0x01000000,
						Producer:    cfg.Server.ID,
						GenesisHash: cfg.Genesis.SignedHeader.BlockHash,
						ParentHash:  cfg.Genesis.SignedHeader.BlockHash,
					},
					Signee: pub,
				},
			},
			stopCh: make(chan struct{}),
		},
		state: &State{},
	}

	err = chain.db.View(func(tx *bolt.Tx) (err error) {
		// Read state struct
		meta := tx.Bucket(metaBucket[:])
		err = chain.state.UnmarshalBinary(meta.Get(metaStateKey))

		if err != nil {
			return err
		}

		// Read blocks and rebuild memory index
		var last *blockNode
		var index int32
		blocks := meta.Bucket(metaBlockIndexBucket)
		nodes := make([]blockNode, blocks.Stats().KeyN)

		if err = blocks.ForEach(func(k, v []byte) (err error) {
			block := &ct.Block{}

			if err = block.UnmarshalBinary(v); err != nil {
				return
			}

			log.Debugf("Read new block: %+v", block)
			parent := (*blockNode)(nil)

			if last == nil {
				if err = block.SignedHeader.VerifyAsGenesis(); err != nil {
					return
				}
			} else if block.SignedHeader.ParentHash.IsEqual(&last.hash) {
				if err = block.SignedHeader.Verify(); err != nil {
					return
				}

				parent = last
			} else {
				parent = chain.bi.LookupNode(&block.SignedHeader.BlockHash)

				if parent == nil {
					return ErrParentNotFound
				}
			}

			nodes[index].initBlockNode(block, parent)
			last = &nodes[index]
			index++
			return
		}); err != nil {
			return
		}

		// Read queries and rebuild memory index
		heights := meta.Bucket(metaHeightIndexBucket)
		resp := &wt.SignedResponseHeader{}
		ack := &wt.SignedAckHeader{}

		if err = heights.ForEach(func(k, v []byte) (err error) {
			h := KeyToHeight(k)

			if resps := heights.Bucket(k).Bucket(
				metaResponseIndexBucket); resps != nil {
				if err = resps.ForEach(func(k []byte, v []byte) (err error) {
					if err = resp.UnmarshalBinary(v); err != nil {
						return
					}

					return chain.qi.AddResponse(h, resp)
				}); err != nil {
					return
				}
			}

			if acks := heights.Bucket(k).Bucket(metaAckIndexBucket); acks != nil {
				if err = acks.ForEach(func(k []byte, v []byte) (err error) {
					if err = ack.UnmarshalBinary(v); err != nil {
						return
					}

					return chain.qi.AddAck(h, ack)
				}); err != nil {
					return
				}
			}

			return
		}); err != nil {
			return
		}

		return
	})

	return
}

// PushBlock pushes the signed block header to extend the current main chain.
func (c *Chain) PushBlock(b *ct.Block) (err error) {
	// Prepare and encode
	h := c.cfg.GetHeightFromTime(b.SignedHeader.Timestamp)
	node := newBlockNode(b, c.state.node)
	state := &State{
		node:   node,
		Head:   node.hash,
		Height: node.height,
	}
	var encBlock, encState []byte

	if encBlock, err = b.MarshalBinary(); err != nil {
		return
	}

	if encState, err = state.MarshalBinary(); err != nil {
		return
	}

	// Update in transaction
	return c.db.Update(func(tx *bolt.Tx) (err error) {
		if err = tx.Bucket(metaBucket[:]).Put(metaStateKey, encState); err != nil {
			return
		}

		if err = tx.Bucket(metaBucket[:]).Bucket(metaBlockIndexBucket).Put(
			node.indexKey(), encBlock); err != nil {
			return
		}

		c.state = state
		c.bi.AddBlock(node)
		c.qi.SetSignedBlock(h, b)

		return
	})
}

func ensureHeight(tx *bolt.Tx, k []byte) (hb *bolt.Bucket, err error) {
	b := tx.Bucket(metaBucket[:]).Bucket(metaHeightIndexBucket)

	if hb = b.Bucket(k); hb == nil {
		// Create and initialize bucket in new height
		if hb, err = b.CreateBucket(k); err != nil {
			return
		}

		if _, err = hb.CreateBucket(metaRequestIndexBucket); err != nil {
			return
		}

		if _, err = hb.CreateBucket(metaResponseIndexBucket); err != nil {
			return
		}

		if _, err = hb.CreateBucket(metaAckIndexBucket); err != nil {
			return
		}
	}

	return
}

// PushResponedQuery pushes a responsed, signed and verified query into the chain.
func (c *Chain) PushResponedQuery(resp *wt.SignedResponseHeader) (err error) {
	h := c.cfg.GetHeightFromTime(resp.Request.Timestamp)
	k := HeightToKey(h)
	var enc []byte

	if enc, err = resp.MarshalBinary(); err != nil {
		return
	}

	return c.db.Update(func(tx *bolt.Tx) (err error) {
		heightBucket, err := ensureHeight(tx, k)

		if err != nil {
			return
		}

		if err = heightBucket.Bucket(metaResponseIndexBucket).Put(
			resp.HeaderHash[:], enc); err != nil {
			return
		}

		// Always put memory changes which will not be affected by rollback after DB operations
		return c.qi.AddResponse(h, resp)
	})
}

// PushAckedQuery pushed a acknowledged, signed and verified query into the chain.
func (c *Chain) PushAckedQuery(ack *wt.SignedAckHeader) (err error) {
	h := c.cfg.GetHeightFromTime(ack.SignedResponseHeader().Timestamp)
	k := HeightToKey(h)
	var enc []byte

	if enc, err = ack.MarshalBinary(); err != nil {
		return
	}

	return c.db.Update(func(tx *bolt.Tx) (err error) {
		b, err := ensureHeight(tx, k)

		if err != nil {
			return
		}

		// TODO(leventeliu): this doesn't seem right to use an error to detect key existence.
		if err = b.Bucket(metaAckIndexBucket).Put(
			ack.HeaderHash[:], enc,
		); err != nil && err != bolt.ErrIncompatibleValue {
			return
		}

		// Always put memory changes which will not be affected by rollback after DB operations
		if err = c.qi.AddAck(h, ack); err != nil {
			return
		}

		if c.IsMyTurn() {
			c.rt.pendingBlock.PushAckedQuery(&ack.HeaderHash)
		}

		return
	})
}

// CheckAndPushNewBlock implements ChainRPCServer.CheckAndPushNewBlock.
func (c *Chain) CheckAndPushNewBlock(block *ct.Block) (err error) {
	// Pushed block must extend the best chain
	if !block.SignedHeader.ParentHash.IsEqual(&c.state.Head) {
		return ErrInvalidBlock
	}

	// TODO(leventeliu): verify that block.SignedHeader.Producer is the rightful producer of the
	// block.

	// Check block existence
	if c.bi.HasBlock(&block.SignedHeader.BlockHash) {
		return ErrBlockExists
	}

	// Block must produced within [start, end)
	h := c.cfg.GetHeightFromTime(block.SignedHeader.Timestamp)

	if h != c.state.Height+1 {
		return ErrBlockTimestampOutOfPeriod
	}

	// Check queries
	for _, q := range block.Queries {
		var ok bool

		if ok, err = c.qi.CheckAckFromBlock(h, &block.SignedHeader.BlockHash, q); err != nil {
			return
		}

		if !ok {
			// TODO(leventeliu): call RPC.FetchAckedQuery from block.SignedHeader.Producer
		}
	}

	// Verify block signatures
	if err = block.Verify(); err != nil {
		return
	}

	return c.PushBlock(block)
}

func (c *Chain) queryTimeIsExpired(t time.Time) bool {
	// Checking query expiration for the pending block, whose height is c.rt.NextHeight:
	//
	//     TimeLived = c.rt.NextHeight - c.cfg.GetHeightFromTime(t)
	//
	// Return true if:  TTL < TimeLived.
	//
	// NOTE(leventeliu): as a result, a TTL=1 requires any query to be acknowledged and received
	// immediately.
	// Consider the case that a query has happended right before period h, which has height h.
	// If its ACK+Roundtrip time>0, it will be seemed as acknowledged in period h+1, or even later.
	// So, period h+1 has NextHeight h+2, and TimeLived of this query will be 2 at that time - it
	// has expired.
	//
	return c.cfg.GetHeightFromTime(t) < c.rt.NextTurn-c.cfg.QueryTTL
}

// VerifyAndPushResponsedQuery verifies a responsed and signed query, and pushed it if valid.
func (c *Chain) VerifyAndPushResponsedQuery(resp *wt.SignedResponseHeader) (err error) {
	// TODO(leventeliu): check resp.
	if c.queryTimeIsExpired(resp.Timestamp) {
		return ErrQueryExpired
	}

	if err = resp.Verify(); err != nil {
		return
	}

	return c.PushResponedQuery(resp)
}

// VerifyAndPushAckedQuery verifies a acknowledged and signed query, and pushed it if valid.
func (c *Chain) VerifyAndPushAckedQuery(ack *wt.SignedAckHeader) (err error) {
	// TODO(leventeliu): check ack.
	if c.queryTimeIsExpired(ack.SignedResponseHeader().Timestamp) {
		return ErrQueryExpired
	}

	if err = ack.Verify(); err != nil {
		return
	}

	return c.PushAckedQuery(ack)
}

// IsMyTurn returns whether it's my turn to produce block or not.
//
// TODO(leventliu): need implementation.
func (c *Chain) IsMyTurn() bool {
	return c.isMyTurn
}

// ProduceBlock prepares, signs and advises the pending block to the orther peers.
func (c *Chain) ProduceBlock(parent hash.Hash, now time.Time) (err error) {
	// TODO(leventeliu): remember to initialize local key store somewhere.
	priv, err := kms.GetLocalPrivateKey()

	if err != nil {
		return
	}

	// Sign pending block
	c.rt.pendingBlock.SignedHeader.ParentHash = c.state.Head
	c.rt.pendingBlock.SignedHeader.Timestamp = now
	c.rt.pendingBlock.SignedHeader.ParentHash = parent

	if err = c.rt.pendingBlock.PackAndSignBlock(priv); err != nil {
		return
	}

	// TODO(leventeliu): advise new block
	// ...

	if err = c.PushBlock(c.rt.pendingBlock); err != nil {
		return
	}

	return
}

// RunCurrentTurn does the check and runs block producing if its my turn.
func (c *Chain) RunCurrentTurn(now time.Time) {
	defer c.rt.SetNextTurn()

	if !c.IsMyTurn() {
		return
	}

	if err := c.ProduceBlock(c.state.Head, now); err != nil {
		c.Stop()
	}
}

// BlockProducingCycle runs a constantly check for a short time resolution.
func (c *Chain) BlockProducingCycle() {
	for {
		select {
		case <-c.rt.stopCh:
			return
		default:
			if t, d := c.rt.TillNextTurn(); d > 0 {
				time.Sleep(d)
			} else {
				c.RunCurrentTurn(t)
			}
		}
	}
}

// Sync synchronizes blocks and queries from the other peers.
//
// TODO(leventeliu): need implementation.
func (c *Chain) Sync() error {
	return nil
}

// Start starts the main process of the sql-chain.
func (c *Chain) Start() (err error) {
	if err = c.Sync(); err != nil {
		return
	}

	c.BlockProducingCycle()
	return
}

// Stop stops the main process of the sql-chain.
func (c *Chain) Stop() {
	c.rt.Stop()
}

// FetchBlock fetches the block at specified height from local cache.
func (c *Chain) FetchBlock(height int32) (b *ct.Block, err error) {
	if n := c.state.node.ancestor(height); n != nil {
		k := n.indexKey()

		err = c.db.View(func(tx *bolt.Tx) (err error) {
			v := tx.Bucket(metaBucket[:]).Bucket(metaBlockIndexBucket).Get(k)
			err = b.UnmarshalBinary(v)
			return
		})
	}

	return
}

// FetchAckedQuery fetches the acknowledged query from local cache.
func (c *Chain) FetchAckedQuery(height int32, header *hash.Hash) (
	ack *wt.SignedAckHeader, err error,
) {
	return c.qi.GetAck(height, header)
}
