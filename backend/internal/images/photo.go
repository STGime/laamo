package images

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"image"
	"image/jpeg"

	xdraw "golang.org/x/image/draw"
	_ "golang.org/x/image/webp" // phones and browsers send WebP too
)

const (
	// MaxPhotoSide: a stored photo's long side, in pixels. Small print on a
	// till receipt stays readable; a phone's 12 megapixels would cost more
	// to read and store without telling the AI more.
	MaxPhotoSide = 1800
	// maxPhotoPixels: bigger pictures aren't decoded at all (memory).
	maxPhotoPixels = 60_000_000
)

// ErrNotAPhoto: the file isn't a JPEG, PNG or WebP picture we can read.
var ErrNotAPhoto = errors.New("images: not a JPEG, PNG or WebP picture")

// Photo turns a photo someone took or emailed into the JPEG we keep: the
// right way up (phones store how the camera was held instead of turning the
// picture), at most MaxPhotoSide on its long side and at most MaxBytes.
func Photo(data []byte) ([]byte, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotAPhoto, err)
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width*cfg.Height > maxPhotoPixels {
		return nil, fmt.Errorf("%w: %d×%d pixels", ErrNotAPhoto, cfg.Width, cfg.Height)
	}
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrNotAPhoto, err)
	}
	b := src.Bounds()
	w, h := b.Dx(), b.Dy()
	if long := max(w, h); long > MaxPhotoSide {
		w, h = max(1, w*MaxPhotoSide/long), max(1, h*MaxPhotoSide/long)
	}
	// Onto white: a see-through PNG would turn black as a JPEG.
	small := image.NewRGBA(image.Rect(0, 0, w, h))
	xdraw.Draw(small, small.Bounds(), image.White, image.Point{}, xdraw.Src)
	xdraw.CatmullRom.Scale(small, small.Bounds(), src, b, xdraw.Over, nil)
	upright := turn(small, orientation(data))
	for _, q := range []int{85, 75, 65, 50, 35} {
		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, upright, &jpeg.Options{Quality: q}); err != nil {
			return nil, err
		}
		if buf.Len() <= MaxBytes {
			return buf.Bytes(), nil
		}
	}
	return nil, fmt.Errorf("images: the photo is still over %d bytes", MaxBytes)
}

// turn applies an EXIF orientation (1 to 8) so the picture is upright.
func turn(src *image.RGBA, o int) *image.RGBA {
	if o < 2 || o > 8 {
		return src
	}
	w, h := src.Bounds().Dx(), src.Bounds().Dy()
	dw, dh := w, h
	if o >= 5 {
		dw, dh = h, w
	}
	dst := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := range dh {
		for x := range dw {
			var sx, sy int
			switch o {
			case 2: // mirrored
				sx, sy = w-1-x, y
			case 3: // upside down
				sx, sy = w-1-x, h-1-y
			case 4:
				sx, sy = x, h-1-y
			case 5:
				sx, sy = y, x
			case 6: // camera held upright: turn 90° clockwise
				sx, sy = y, h-1-x
			case 7:
				sx, sy = w-1-y, h-1-x
			case 8: // turn 90° counter-clockwise
				sx, sy = w-1-y, x
			}
			dst.SetRGBA(x, y, src.RGBAAt(sx, sy))
		}
	}
	return dst
}

// orientation reads the EXIF orientation of a JPEG (1 = upright, also when
// there's none or the file isn't a JPEG).
func orientation(data []byte) int {
	if len(data) < 4 || data[0] != 0xFF || data[1] != 0xD8 {
		return 1
	}
	for i := 2; i+4 <= len(data); {
		if data[i] != 0xFF {
			return 1
		}
		marker := data[i+1]
		if marker == 0xDA || marker == 0xD9 { // picture data starts: no EXIF before it
			return 1
		}
		size := int(binary.BigEndian.Uint16(data[i+2:]))
		if size < 2 || i+2+size > len(data) {
			return 1
		}
		seg := data[i+4 : i+2+size]
		if marker == 0xE1 && bytes.HasPrefix(seg, []byte("Exif\x00\x00")) {
			return tiffOrientation(seg[6:])
		}
		i += 2 + size
	}
	return 1
}

func tiffOrientation(t []byte) int {
	if len(t) < 8 {
		return 1
	}
	var bo binary.ByteOrder
	switch string(t[:2]) {
	case "II":
		bo = binary.LittleEndian
	case "MM":
		bo = binary.BigEndian
	default:
		return 1
	}
	off := int(bo.Uint32(t[4:]))
	if off < 8 || off+2 > len(t) {
		return 1
	}
	n := int(bo.Uint16(t[off:]))
	for k := range n {
		e := off + 2 + k*12
		if e+12 > len(t) {
			return 1
		}
		if bo.Uint16(t[e:]) == 0x0112 { // Orientation
			if v := int(bo.Uint16(t[e+8:])); v >= 1 && v <= 8 {
				return v
			}
			return 1
		}
	}
	return 1
}
