package embeddings

import (
	"bufio"
	"crypto/sha256"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

// StoreMeta identifies the embedding space a Store holds. Vectors from
// different models, task types or dimensions are not comparable, so a Store
// refuses to open with metadata other than what it was created with.
type StoreMeta struct {
	Model    string `json:"model"`
	TaskType string `json:"task_type"`
	Dims     int    `json:"dims"`
}

// Store is a content-addressed vector cache: each vector is keyed by the
// SHA-256 of the exact text that was embedded. A vector can therefore never
// be attached to the wrong text, changed texts are simply missing (never
// silently reused), and identical texts are embedded once.
//
// On disk: meta.json plus append-only *.seg files, each written atomically
// after a batch is embedded, so an interrupted run keeps everything it paid for.
// Segment format: "JEVS" magic, dims uint32, count uint32, then per vector a
// 32-byte text hash followed by dims little-endian float32 values.
type Store struct {
	dir  string
	meta StoreMeta
	vecs map[[32]byte][]float32
}

const segMagic = "JEVS"

// OpenStore opens (or creates) the store in dir and loads every segment.
func OpenStore(dir string, meta StoreMeta) (*Store, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	metaPath := filepath.Join(dir, "meta.json")
	data, err := os.ReadFile(metaPath)
	switch {
	case errors.Is(err, fs.ErrNotExist):
		b, _ := json.MarshalIndent(meta, "", "  ")
		if err := writeFileAtomic(metaPath, b); err != nil {
			return nil, err
		}
	case err != nil:
		return nil, err
	default:
		var have StoreMeta
		if err := json.Unmarshal(data, &have); err != nil {
			return nil, fmt.Errorf("embeddings: parsing %s: %w", metaPath, err)
		}
		if have != meta {
			return nil, fmt.Errorf("embeddings: store %s holds model=%q task=%q dims=%d, asked for model=%q task=%q dims=%d",
				dir, have.Model, have.TaskType, have.Dims, meta.Model, meta.TaskType, meta.Dims)
		}
	}

	s := &Store{dir: dir, meta: meta, vecs: map[[32]byte][]float32{}}
	segs, err := filepath.Glob(filepath.Join(dir, "*.seg"))
	if err != nil {
		return nil, err
	}
	sort.Strings(segs)
	for _, p := range segs {
		if err := s.loadSegment(p); err != nil {
			return nil, err
		}
	}
	return s, nil
}

// Len returns the number of stored vectors.
func (s *Store) Len() int { return len(s.vecs) }

// Get returns the vector stored for text.
func (s *Store) Get(text string) ([]float64, bool) {
	v, ok := s.vecs[sha256.Sum256([]byte(text))]
	if !ok {
		return nil, false
	}
	out := make([]float64, len(v))
	for i, x := range v {
		out[i] = float64(x)
	}
	return out, true
}

// Missing returns the distinct texts that have no stored vector, in order.
func (s *Store) Missing(texts []string) []string {
	seen := map[[32]byte]bool{}
	var out []string
	for _, t := range texts {
		h := sha256.Sum256([]byte(t))
		if _, ok := s.vecs[h]; ok || seen[h] {
			continue
		}
		seen[h] = true
		out = append(out, t)
	}
	return out
}

// Add stores vectors for texts and persists them as a new segment before
// returning, so a crash right after never loses a paid batch.
func (s *Store) Add(texts []string, vecs [][]float64) error {
	if len(texts) != len(vecs) {
		return fmt.Errorf("embeddings: store add: %d texts, %d vectors", len(texts), len(vecs))
	}
	hashes := make([][32]byte, len(texts))
	f32 := make([][]float32, len(vecs))
	for i, v := range vecs {
		if len(v) != s.meta.Dims {
			return fmt.Errorf("embeddings: store add: vector %d has %d dims, want %d", i, len(v), s.meta.Dims)
		}
		hashes[i] = sha256.Sum256([]byte(texts[i]))
		f32[i] = make([]float32, len(v))
		for j, x := range v {
			f32[i][j] = float32(x)
		}
	}

	path := filepath.Join(s.dir, fmt.Sprintf("%020d.seg", time.Now().UnixNano()))
	if err := s.writeSegment(path, hashes, f32); err != nil {
		return err
	}
	for i, h := range hashes {
		s.vecs[h] = f32[i]
	}
	return nil
}

func (s *Store) writeSegment(path string, hashes [][32]byte, vecs [][]float32) error {
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	w := bufio.NewWriter(f)
	w.WriteString(segMagic)
	binary.Write(w, binary.LittleEndian, [2]uint32{uint32(s.meta.Dims), uint32(len(vecs))})
	buf := make([]byte, 4)
	for i, h := range hashes {
		w.Write(h[:])
		for _, x := range vecs[i] {
			binary.LittleEndian.PutUint32(buf, math.Float32bits(x))
			w.Write(buf)
		}
	}
	if err := w.Flush(); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func (s *Store) loadSegment(path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	r := bufio.NewReader(f)

	magic := make([]byte, 4)
	var hdr [2]uint32
	if _, err := io.ReadFull(r, magic); err != nil || string(magic) != segMagic {
		return fmt.Errorf("embeddings: %s is not a vector segment", path)
	}
	if err := binary.Read(r, binary.LittleEndian, &hdr); err != nil {
		return fmt.Errorf("embeddings: reading header of %s: %w", path, err)
	}
	dims, count := int(hdr[0]), int(hdr[1])
	if dims != s.meta.Dims {
		return fmt.Errorf("embeddings: %s has dims %d, store has %d", path, dims, s.meta.Dims)
	}

	rec := make([]byte, 32+4*dims)
	for i := 0; i < count; i++ {
		if _, err := io.ReadFull(r, rec); err != nil {
			return fmt.Errorf("embeddings: %s truncated at vector %d: %w", path, i, err)
		}
		var h [32]byte
		copy(h[:], rec[:32])
		v := make([]float32, dims)
		for j := range v {
			v[j] = math.Float32frombits(binary.LittleEndian.Uint32(rec[32+4*j:]))
		}
		s.vecs[h] = v
	}
	if _, err := r.ReadByte(); err != io.EOF {
		return fmt.Errorf("embeddings: %s has trailing data", path)
	}
	return nil
}

// Reduce truncates v to its first dims values and L2-normalizes the result
// (Matryoshka-style: gemini-embedding-001's lower-dimensional outputs are the
// leading values of the full vector). dims <= 0 keeps the full length.
func Reduce(v []float64, dims int) []float64 {
	if dims <= 0 || dims > len(v) {
		dims = len(v)
	}
	out := make([]float64, dims)
	norm := 0.0
	for i := range out {
		out[i] = v[i]
		norm += v[i] * v[i]
	}
	if norm == 0 {
		return out
	}
	norm = math.Sqrt(norm)
	for i := range out {
		out[i] /= norm
	}
	return out
}

func writeFileAtomic(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
