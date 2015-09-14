//
// quick hack based in https://github.com/kelseyhightower/cpic/image
//

//Package image is a set of helpers to manìpulate and refactor CoreOS's pxe
//payloads
package image

import (
	"bytes"
	"compress/gzip"
	"io"
	"time"

	"github.com/deoxxa/gocpio"
)

type Reader struct {
	z *gzip.Reader
	c *cpio.Reader
}

type Writer struct {
	z *gzip.Writer
	c *cpio.Writer
}

func NewReader(r io.Reader) (*Reader, error) {
	z, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	return &Reader{z, cpio.NewReader(z)}, nil
}

func (r *Reader) Close() (err error) {
	if err = r.z.Close(); err != nil {
		return
	}
	return
}

func NewWriter(w io.Writer) (*Writer, error) {
	z := gzip.NewWriter(w)
	return &Writer{z, cpio.NewWriter(z)}, nil
}

func (w *Writer) Close() (err error) {
	if err = w.c.Close(); err != nil {
		return
	}
	if err = w.z.Close(); err != nil {
		return
	}
	return
}

func (w *Writer) Write(p []byte) (int, error) {
	return w.c.Write(p)
}

func (w *Writer) WriteHeader(hdr *cpio.Header) error {
	return w.c.WriteHeader(hdr)
}

func Copy(dst *Writer, src *Reader) (err error) {
	for {
		var h *cpio.Header
		if h, err = src.c.Next(); err != nil {
			return
		}
		if h.IsTrailer() {
			break
		}
		if h.Type == cpio.TYPE_DIR {
			if h.Name == "." {
				continue
			}
			if err = dst.c.WriteHeader(h); err != nil {
				return
			}
			continue
		}
		if err = dst.c.WriteHeader(h); err != nil {
			return err
		}
		if _, err = io.Copy(dst.c, src.c); err != nil {
			return
		}
	}
	return
}

// Appends a new file with the given payload to the cpio
// archive being refactored
func (w *Writer) WriteToFile(content *bytes.Buffer,
	name string, mode int64) (err error) {
	h := cpio.Header{
		Name:  name,
		Mode:  mode,
		Mtime: time.Now().Unix(),
		Size:  int64(content.Len()),
		Type:  cpio.TYPE_REG,
	}
	if err = w.WriteHeader(&h); err != nil {
		return
	}
	if _, err = io.Copy(w, content); err != nil {
		return
	}
	return
}

// Appends a new directory with the given payload to the cpio archive being
// refactored
//
// It isn't recursive so if one intends to append a "/foo/bar" directory this
// should be called first for "foo" and only after for "foo/bar"
func (w *Writer) WriteDir(name string, mode int64) (err error) {
	h := cpio.Header{
		Name:  name,
		Mode:  mode,
		Mtime: time.Now().Unix(),
		Type:  cpio.TYPE_DIR,
	}
	if err = w.WriteHeader(&h); err != nil {
		return
	}
	return
}
