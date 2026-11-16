package images

import (
	"bytes"
	"image"
	"image/jpeg"
	"testing"
)

func TestShrink(t *testing.T) {
	// A detailed 1024² picture (gradients and fine texture): over 1 MB as a
	// top-quality JPEG, like the drawings models send.
	img := image.NewRGBA(image.Rect(0, 0, 1024, 1024))
	for y := range 1024 {
		for x := range 1024 {
			n := byte((x*31 + y*17 + x*y) % 23)
			i := img.PixOffset(x, y)
			img.Pix[i], img.Pix[i+1], img.Pix[i+2], img.Pix[i+3] = byte(x/4)+n, byte(y/4)+n, byte((x+y)/8)+n, 255
		}
	}
	var buf bytes.Buffer
	_ = jpeg.Encode(&buf, img, &jpeg.Options{Quality: 100})
	if buf.Len() <= MaxBytes {
		t.Fatalf("test picture only %d bytes", buf.Len())
	}
	out, err := Shrink(buf.Bytes(), MaxBytes)
	if err != nil || len(out) > MaxBytes || !bytes.HasPrefix(out, []byte{0xFF, 0xD8}) {
		t.Fatalf("shrink: %v %d bytes", err, len(out))
	}
	small := []byte{0xFF, 0xD8, 1, 2}
	if out, _ := Shrink(small, MaxBytes); !bytes.Equal(out, small) {
		t.Fatal("a small JPEG was changed")
	}
}
