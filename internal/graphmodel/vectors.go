package graphmodel

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"math"
	"os"
)

// SaveVectors writes embeddings as little-endian float32: a header of
// (count, dim) as uint32 followed by count*dim values. Large corpora
// (44k × 768) take ~136 MB this way versus several GB as JSON. Precision
// drops to float32, which is what the embedding API returns anyway.
func SaveVectors(path string, vecs [][]float64) error {
	dim := 0
	if len(vecs) > 0 {
		dim = len(vecs[0])
	}
	f, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("graphmodel: creating %s: %w", path, err)
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	if err := binary.Write(w, binary.LittleEndian, [2]uint32{uint32(len(vecs)), uint32(dim)}); err != nil {
		return err
	}
	buf := make([]byte, 4)
	for i, v := range vecs {
		if len(v) != dim {
			return fmt.Errorf("graphmodel: vector %d has dim %d, want %d", i, len(v), dim)
		}
		for _, x := range v {
			binary.LittleEndian.PutUint32(buf, math.Float32bits(float32(x)))
			if _, err := w.Write(buf); err != nil {
				return err
			}
		}
	}
	if err := w.Flush(); err != nil {
		return err
	}
	return f.Close()
}

// LoadVectors reads embeddings written by SaveVectors.
func LoadVectors(path string) ([][]float64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("graphmodel: opening %s: %w", path, err)
	}
	defer f.Close()

	r := bufio.NewReader(f)
	var hdr [2]uint32
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return nil, fmt.Errorf("graphmodel: reading header of %s: %w", path, err)
	}
	count, dim := int(hdr[0]), int(hdr[1])

	vecs := make([][]float64, count)
	buf := make([]byte, 4*dim)
	for i := range vecs {
		if _, err := io.ReadFull(r, buf); err != nil {
			return nil, fmt.Errorf("graphmodel: reading vector %d of %s: %w", i, path, err)
		}
		v := make([]float64, dim)
		for j := range v {
			v[j] = float64(math.Float32frombits(binary.LittleEndian.Uint32(buf[4*j:])))
		}
		vecs[i] = v
	}
	return vecs, nil
}
