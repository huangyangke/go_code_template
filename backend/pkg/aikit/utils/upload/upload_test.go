package upload

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestCalcCRC64(t *testing.T) {
	tests := []struct {
		name  string
		input []byte
	}{
		{"empty", []byte{}},
		{"hello", []byte("hello world")},
		{"binary", []byte{0x00, 0xff, 0x01, 0xfe}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := CalcCRC64(tt.input)
			assert.NotEmpty(t, result)

			result2 := CalcCRC64(tt.input)
			assert.Equal(t, result, result2)
		})
	}
}

func TestCalcCRC64_DifferentInputs(t *testing.T) {
	a := CalcCRC64([]byte("hello"))
	b := CalcCRC64([]byte("world"))
	assert.NotEqual(t, a, b)
}

func TestDetectExt_PNG(t *testing.T) {
	data := []byte("\x89PNG\r\n\x1a\nrest-of-data")
	assert.Equal(t, "png", DetectExt(data))
}

func TestDetectExt_JPG(t *testing.T) {
	data := []byte("\xff\xd8\xff\xe0some-jpeg-data")
	assert.Equal(t, "jpg", DetectExt(data))
}

func TestDetectExt_GIF87a(t *testing.T) {
	data := []byte("GIF87a" + "rest-of-data")
	assert.Equal(t, "gif", DetectExt(data))
}

func TestDetectExt_GIF89a(t *testing.T) {
	data := []byte("GIF89a" + "rest-of-data")
	assert.Equal(t, "gif", DetectExt(data))
}

func TestDetectExt_BMP(t *testing.T) {
	data := []byte("BMrest-of-data-here")
	assert.Equal(t, "bmp", DetectExt(data))
}

func TestDetectExt_BMPShortHeader(t *testing.T) {
	data := []byte("BM")
	assert.Equal(t, "bmp", DetectExt(data))
}

func TestDetectExt_WAV(t *testing.T) {
	data := []byte("RIFF\x00\x00\x00\x00WAVEmore")
	assert.Equal(t, "wav", DetectExt(data))
}

func TestDetectExt_WEBP(t *testing.T) {
	data := []byte("RIFF\x00\x00\x00\x00WEBPmore")
	assert.Equal(t, "webp", DetectExt(data))
}

func TestDetectExt_MP3_ID3(t *testing.T) {
	data := []byte("ID3\x04\x00\x00\x00\x00\x00\x00")
	assert.Equal(t, "mp3", DetectExt(data))
}

func TestDetectExt_MP3_SyncByte(t *testing.T) {
	data := []byte("\xff\xfb\x90\x00some-mp3-data")
	assert.Equal(t, "mp3", DetectExt(data))
}

func TestDetectExt_Unknown(t *testing.T) {
	data := []byte{0x00, 0x01, 0x02, 0x03, 0xfe, 0xff}
	assert.Equal(t, "unknown", DetectExt(data))
}

func TestDetectExt_TooShort(t *testing.T) {
	assert.Equal(t, "unknown", DetectExt([]byte{0x89}))
	assert.Equal(t, "unknown", DetectExt([]byte{}))
	assert.Equal(t, "unknown", DetectExt([]byte{0x89, 0x50}))
}

func TestDetectExt_JSON(t *testing.T) {
	data := []byte("{\"name\":\"alice\"}")
	assert.Equal(t, "json", DetectExt(data))
}

func TestDetectExt_TXT(t *testing.T) {
	data := []byte("plain text content")
	assert.Equal(t, "txt", DetectExt(data))
}

func TestExtOf_WithFileName(t *testing.T) {
	assert.Equal(t, "jpg", extOf("photo.jpg", nil))
	assert.Equal(t, "png", extOf("image.png", nil))
}

func TestExtOf_FallsBackToDetect(t *testing.T) {
	data := []byte("\x89PNG\r\n\x1a\nrest")
	assert.Equal(t, "png", extOf("", data))
}

func TestNew_PanicsOnMissingFields(t *testing.T) {
	assert.Panics(t, func() {
		New(&Config{})
	})
	assert.Panics(t, func() {
		New(&Config{Addr: "http://example.com"})
	})
	assert.Panics(t, func() {
		New(&Config{Addr: "http://example.com", PlatName: "test"})
	})
}

func TestNew_ValidConfig(t *testing.T) {
	u := New(&Config{
		Addr:     "http://upload.example.com",
		PlatName: "test",
		PlatKey:  "key123",
		Uploader: "admin",
	})
	assert.NotNil(t, u)
}
