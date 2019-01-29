package types

// Code generated by github.com/CovenantSQL/HashStablePack DO NOT EDIT.

import (
	hsp "github.com/CovenantSQL/HashStablePack/marshalhash"
)

// MarshalHash marshals for hash
func (z *SignedUpdateServiceHeader) MarshalHash() (o []byte, err error) {
	var b []byte
	o = hsp.Require(b, z.Msgsize())
	// map header, size 2
	o = append(o, 0x82)
	if oTemp, err := z.DefaultHashSignVerifierImpl.MarshalHash(); err != nil {
		return nil, err
	} else {
		o = hsp.AppendBytes(o, oTemp)
	}
	// map header, size 2
	o = append(o, 0x82)
	o = hsp.AppendInt32(o, int32(z.UpdateServiceHeader.Op))
	if oTemp, err := z.UpdateServiceHeader.Instance.MarshalHash(); err != nil {
		return nil, err
	} else {
		o = hsp.AppendBytes(o, oTemp)
	}
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *SignedUpdateServiceHeader) Msgsize() (s int) {
	s = 1 + 28 + z.DefaultHashSignVerifierImpl.Msgsize() + 20 + 1 + 3 + hsp.Int32Size + 9 + z.UpdateServiceHeader.Instance.Msgsize()
	return
}

// MarshalHash marshals for hash
func (z *UpdateService) MarshalHash() (o []byte, err error) {
	var b []byte
	o = hsp.Require(b, z.Msgsize())
	// map header, size 2
	o = append(o, 0x82)
	if oTemp, err := z.Envelope.MarshalHash(); err != nil {
		return nil, err
	} else {
		o = hsp.AppendBytes(o, oTemp)
	}
	if oTemp, err := z.Header.MarshalHash(); err != nil {
		return nil, err
	} else {
		o = hsp.AppendBytes(o, oTemp)
	}
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *UpdateService) Msgsize() (s int) {
	s = 1 + 9 + z.Envelope.Msgsize() + 7 + z.Header.Msgsize()
	return
}

// MarshalHash marshals for hash
func (z *UpdateServiceHeader) MarshalHash() (o []byte, err error) {
	var b []byte
	o = hsp.Require(b, z.Msgsize())
	// map header, size 2
	o = append(o, 0x82)
	if oTemp, err := z.Instance.MarshalHash(); err != nil {
		return nil, err
	} else {
		o = hsp.AppendBytes(o, oTemp)
	}
	o = hsp.AppendInt32(o, int32(z.Op))
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z *UpdateServiceHeader) Msgsize() (s int) {
	s = 1 + 9 + z.Instance.Msgsize() + 3 + hsp.Int32Size
	return
}

// MarshalHash marshals for hash
func (z UpdateServiceResponse) MarshalHash() (o []byte, err error) {
	var b []byte
	o = hsp.Require(b, z.Msgsize())
	// map header, size 0
	o = append(o, 0x80)
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z UpdateServiceResponse) Msgsize() (s int) {
	s = 1
	return
}

// MarshalHash marshals for hash
func (z UpdateType) MarshalHash() (o []byte, err error) {
	var b []byte
	o = hsp.Require(b, z.Msgsize())
	o = hsp.AppendInt32(o, int32(z))
	return
}

// Msgsize returns an upper bound estimate of the number of bytes occupied by the serialized message
func (z UpdateType) Msgsize() (s int) {
	s = hsp.Int32Size
	return
}
