package images

import (
	"bytes"
	"encoding/binary"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"testing"
)

// picture is w×h with a red left half and a blue right half.
func picture(w, h int) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			c := color.RGBA{220, 20, 20, 255}
			if x >= w/2 {
				c = color.RGBA{20, 20, 220, 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	return img
}

func jpegOf(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: 90}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// withOrientation puts an EXIF segment with this orientation after the
// JPEG's start marker.
func withOrientation(data []byte, o uint16) []byte {
	tiff := []byte("MM\x00\x2A\x00\x00\x00\x08")
	tiff = binary.BigEndian.AppendUint16(tiff, 1)      // one entry
	tiff = binary.BigEndian.AppendUint16(tiff, 0x0112) // Orientation
	tiff = binary.BigEndian.AppendUint16(tiff, 3)      // SHORT
	tiff = binary.BigEndian.AppendUint32(tiff, 1)
	tiff = binary.BigEndian.AppendUint16(tiff, o)
	tiff = append(tiff, 0, 0, 0, 0, 0, 0)
	seg := append([]byte("Exif\x00\x00"), tiff...)
	out := []byte{0xFF, 0xD8, 0xFF, 0xE1}
	out = binary.BigEndian.AppendUint16(out, uint16(len(seg)+2)) //nolint:gosec // G115: a 26-byte segment
	out = append(out, seg...)
	return append(out, data[2:]...)
}

func decode(t *testing.T, data []byte) image.Image {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(data))
	if err != nil {
		t.Fatal(err)
	}
	return img
}

func isRed(c color.Color) bool {
	r, _, b, _ := c.RGBA()
	return r > b
}

func TestPhotoShrinksToTheLongSide(t *testing.T) {
	out, err := Photo(jpegOf(t, picture(4000, 3000)))
	if err != nil {
		t.Fatal(err)
	}
	b := decode(t, out).Bounds()
	if b.Dx() != MaxPhotoSide || b.Dy() != 1350 {
		t.Errorf("size = %d×%d, want %d×1350", b.Dx(), b.Dy(), MaxPhotoSide)
	}
	if len(out) > MaxBytes {
		t.Errorf("%d bytes, want at most %d", len(out), MaxBytes)
	}
}

func TestPhotoKeepsASmallPictureItsSize(t *testing.T) {
	out, err := Photo(jpegOf(t, picture(400, 300)))
	if err != nil {
		t.Fatal(err)
	}
	if b := decode(t, out).Bounds(); b.Dx() != 400 || b.Dy() != 300 {
		t.Errorf("size = %d×%d, want 400×300", b.Dx(), b.Dy())
	}
}

// A phone held upright stores a landscape picture marked "turn 90° clockwise":
// what was the left edge (red) must end up on top.
func TestPhotoTurnsUpright(t *testing.T) {
	out, err := Photo(withOrientation(jpegOf(t, picture(400, 200)), 6))
	if err != nil {
		t.Fatal(err)
	}
	img := decode(t, out)
	if b := img.Bounds(); b.Dx() != 200 || b.Dy() != 400 {
		t.Fatalf("size = %d×%d, want 200×400", b.Dx(), b.Dy())
	}
	if !isRed(img.At(100, 20)) || isRed(img.At(100, 380)) {
		t.Error("want red on top and blue at the bottom")
	}

	out, err = Photo(withOrientation(jpegOf(t, picture(400, 200)), 3))
	if err != nil {
		t.Fatal(err)
	}
	if img := decode(t, out); isRed(img.At(20, 100)) || !isRed(img.At(380, 100)) {
		t.Error("upside down: want blue left and red right")
	}
}

func TestPhotoPutsASeeThroughPNGOnWhite(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewNRGBA(image.Rect(0, 0, 50, 50))); err != nil {
		t.Fatal(err)
	}
	out, err := Photo(buf.Bytes())
	if err != nil {
		t.Fatal(err)
	}
	if r, g, b, _ := decode(t, out).At(25, 25).RGBA(); r < 0xF000 || g < 0xF000 || b < 0xF000 {
		t.Errorf("want white, got %x %x %x", r, g, b)
	}
}

func TestPhotoRejectsOtherFiles(t *testing.T) {
	for _, data := range [][]byte{nil, []byte("%PDF-1.7 not a picture"), []byte("hello")} {
		if _, err := Photo(data); !errors.Is(err, ErrNotAPhoto) {
			t.Errorf("Photo(%q) = %v, want ErrNotAPhoto", data, err)
		}
	}
}
