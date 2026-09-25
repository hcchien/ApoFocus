package ingest

import (
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"math"
	"math/bits"
	"os"
	"sort"

	"github.com/rwcarlsen/goexif/exif"
)

const DefaultPHashMaxDistance = 6

var dctCos = func() [8][32]float64 {
	var m [8][32]float64
	for k := 0; k < 8; k++ {
		for n := 0; n < 32; n++ {
			m[k][n] = math.Cos(math.Pi * float64(2*n+1) * float64(k) / 64.0)
		}
	}
	return m
}()

// PHashDistance returns the Hamming distance (0..64) between two 64-bit perceptual hashes.
func PHashDistance(a, b int64) int {
	return bits.OnesCount64(uint64(a ^ b))
}

// ComputePHash computes a 64-bit DCT perceptual hash for standard image formats
// supported by Go's image decoders. Formats requiring external decoders (e.g.
// RAW/TIFF/HEIC) return (_, false) and receive their pHash from the embedding service.
func ComputePHash(path string) (int64, bool) {
	file, err := os.Open(path)
	if err != nil {
		return 0, false
	}
	defer file.Close()

	orientation := 1
	if x, err := exif.Decode(file); err == nil && x != nil {
		if tag, err := x.Get(exif.Orientation); err == nil {
			if o, err := tag.Int(0); err == nil && o >= 1 && o <= 8 {
				orientation = o
			}
		}
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return 0, false
	}
	img, _, err := image.Decode(file)
	if err != nil {
		return 0, false
	}
	return ComputeImagePHash(img, orientation)
}

// ComputeImagePHash computes a 64-bit DCT perceptual hash from an in-memory image.Image.
func ComputeImagePHash(img image.Image, orientation int) (int64, bool) {
	bounds := img.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	if w <= 0 || h <= 0 {
		return 0, false
	}
	var pixels [32][32]float64
	const sub = 4
	for ty := 0; ty < 32; ty++ {
		for tx := 0; tx < 32; tx++ {
			sum := 0.0
			for sySub := 0; sySub < sub; sySub++ {
				v := (float64(ty) + (float64(sySub)+0.5)/float64(sub)) / 32.0
				for sxSub := 0; sxSub < sub; sxSub++ {
					u := (float64(tx) + (float64(sxSub)+0.5)/float64(sub)) / 32.0
					su, sv := orientUV(u, v, orientation)
					sx := bounds.Min.X + int(su*float64(w))
					if sx >= bounds.Max.X {
						sx = bounds.Max.X - 1
					}
					sy := bounds.Min.Y + int(sv*float64(h))
					if sy >= bounds.Max.Y {
						sy = bounds.Max.Y - 1
					}
					r, g, b, _ := img.At(sx, sy).RGBA()
					sum += (0.299*float64(r) + 0.587*float64(g) + 0.114*float64(b)) / 257.0
				}
			}
			pixels[ty][tx] = sum / float64(sub*sub)
		}
	}

	var rowDCT [32][8]float64
	for y := 0; y < 32; y++ {
		for u := 0; u < 8; u++ {
			sum := 0.0
			for x := 0; x < 32; x++ {
				sum += pixels[y][x] * dctCos[u][x]
			}
			rowDCT[y][u] = sum
		}
	}

	var coeffs [64]float64
	idx := 0
	for v := 0; v < 8; v++ {
		for u := 0; u < 8; u++ {
			sum := 0.0
			for y := 0; y < 32; y++ {
				sum += rowDCT[y][u] * dctCos[v][y]
			}
			coeffs[idx] = sum
			idx++
		}
	}

	ac := make([]float64, 63)
	copy(ac, coeffs[1:])
	sort.Float64s(ac)
	if ac[62]-ac[0] < 1.0 {
		return 0, false
	}
	median := ac[31]

	var hash uint64
	for _, c := range coeffs {
		hash <<= 1
		if c > median {
			hash |= 1
		}
	}
	return int64(hash), true
}

func orientUV(u, v float64, orientation int) (float64, float64) {
	switch orientation {
	case 2:
		return 1 - u, v
	case 3:
		return 1 - u, 1 - v
	case 4:
		return u, 1 - v
	case 5:
		return v, u
	case 6:
		return v, 1 - u
	case 7:
		return 1 - v, 1 - u
	case 8:
		return 1 - v, u
	default:
		return u, v
	}
}
