package proto

// Code generated by github.com/CovenantSQL/HashStablePack DO NOT EDIT.

import (
	hsp "github.com/CovenantSQL/HashStablePack/marshalhash"
)

// MarshalHash marshals for hash
func (z *Peers) MarshalHash() (o []byte, err error) {
	var b []byte
	o = hsp.Require(b, z.Msgsize())
	// map header, size 2
	o = append(o, 0x82, 0x82)
	if oTemp, err := z.PeersHeader.MarshalHash(); err != nil {
		return nil, err
	} else {
		o = hsp.AppendBytes(o, oTemp)
	}
	o = append(o, 0x82)
	if oTemp, err := z.DefaultHashSignVerifierImpl.MarshalHash(); err != nil {
		return nil, err
	} else {
		o = hsp.AppendBytes(o, oTemp)
	}
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *Peers) Msgsize() (s int) {
	s = 1 + 12 + z.PeersHeader.Msgsize() + 28 + z.DefaultHashSignVerifierImpl.Msgsize()
	return
}

// MarshalHash marshals for hash
func (z *PeersHeader) MarshalHash() (o []byte, err error) {
	var b []byte
	o = hsp.Require(b, z.Msgsize())
	// map header, size 4
	o = append(o, 0x84, 0x84)
	if oTemp, err := z.Leader.MarshalHash(); err != nil {
		return nil, err
	} else {
		o = hsp.AppendBytes(o, oTemp)
	}
	o = append(o, 0x84)
	o = hsp.AppendArrayHeader(o, uint32(len(z.Servers)))
	for za0001 := range z.Servers {
		if oTemp, err := z.Servers[za0001].MarshalHash(); err != nil {
			return nil, err
		} else {
			o = hsp.AppendBytes(o, oTemp)
		}
	}
	o = append(o, 0x84)
	o = hsp.AppendUint64(o, z.Version)
	o = append(o, 0x84)
	o = hsp.AppendUint64(o, z.Term)
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *PeersHeader) Msgsize() (s int) {
	s = 1 + 7 + z.Leader.Msgsize() + 8 + hsp.ArrayHeaderSize
	for za0001 := range z.Servers {
		s += z.Servers[za0001].Msgsize()
	}
	s += 8 + hsp.Uint64Size + 5 + hsp.Uint64Size
	return
}
