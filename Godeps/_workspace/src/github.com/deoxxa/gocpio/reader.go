package cpio

import (
	"bytes"
	"errors"
	"io"
	"strconv"
)

type Reader struct {
	rd          io.Reader
	unalignment int
	remaining   int
}

func NewReader(r io.Reader) *Reader {
	return &Reader{
		rd: r,
	}
}

var (
	ErrInvalidHeader = errors.New("Did not find valid magic number")
)

const (
	alignTo = 4
)

func (r *Reader) Next() (*Header, error) {
	if r.remaining > 0 {
		if err := r.skip(r.remaining); err != nil {
			return nil, err
		}
	}

	if r.unalignment != 0 {
		if err := r.skip(alignTo - r.unalignment); err != nil {
			return nil, err
		}
	}

	h, err := readHeader(r.rd)
	if err != nil {
		return nil, err
	}

	r.remaining = int(h.Size)

	r.consumed(110 + len(h.Name) + 1)

	if r.unalignment != 0 {
		if err := r.skip(alignTo - r.unalignment); err != nil {
			return nil, err
		}
	}

	return h, nil
}

func (r *Reader) Read(b []byte) (int, error) {
	if r.remaining == 0 {
		return 0, io.EOF
	}

	if len(b) > r.remaining {
		b = b[:r.remaining]
	}

	n, err := r.rd.Read(b)

	if n > 0 {
		r.remaining -= n
		r.consumed(n)
	}

	return n, err
}

func readHeader(rd io.Reader) (*Header, error) {
	b := make([]byte, 110)
	if _, err := io.ReadFull(rd, b); err != nil {
		return nil, err
	}

	if !bytes.HasPrefix(b, []byte("070701")) {
		return nil, ErrInvalidHeader
	}

	h := Header{}

	mode, err := strconv.ParseInt(string(b[14:22]), 16, 64)
	if err != nil {
		return nil, err
	}

	h.Mode = mode & 0xfff
	h.Type = (mode >> 12) & 0xf

	uid, err := strconv.ParseInt(string(b[22:30]), 16, 64)
	if err != nil {
		return nil, err
	}
	h.Uid = int(uid)

	gid, err := strconv.ParseInt(string(b[30:38]), 16, 64)
	if err != nil {
		return nil, err
	}
	h.Gid = int(gid)

	mtime, err := strconv.ParseInt(string(b[46:54]), 16, 64)
	if err != nil {
		return nil, err
	}
	h.Mtime = mtime

	size, err := strconv.ParseInt(string(b[54:62]), 16, 64)
	if err != nil {
		return nil, err
	}
	h.Size = size

	devMajor, err := strconv.ParseInt(string(b[78:86]), 16, 64)
	if err != nil {
		return nil, err
	}
	h.Devmajor = devMajor

	devMinor, err := strconv.ParseInt(string(b[86:94]), 16, 64)
	if err != nil {
		return nil, err
	}
	h.Devminor = devMinor

	l, err := strconv.ParseInt(string(b[94:102]), 16, 64)
	if err != nil {
		return nil, err
	}

	name := make([]byte, l)
	if _, err := io.ReadFull(rd, name); err != nil {
		return nil, err
	}
	// skip the trailing "0" (?)
	h.Name = string(name[:len(name)-1])

	return &h, nil
}

func (r *Reader) skip(n int) error {
	c := 0
	for c < n {
		buf := make([]byte, n-c)

		nr, err := r.rd.Read(buf)
		if err != nil {
			return err
		}

		c += nr
	}

	r.consumed(n)

	return nil
}

func (r *Reader) consumed(n int) {
	r.unalignment = (r.unalignment + n) % alignTo
}
