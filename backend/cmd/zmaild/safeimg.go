package main

import (
	"bytes"
	"errors"
	"image"
	"io"
)

// Image bombs: a tiny PNG/JPEG can declare 40000×40000 px and make image.Decode allocate gigabytes.
// Every decode of untrusted pictures goes through here, which checks the header first.
const (
	maxImageSide   = 12000
	maxImagePixels = 40_000_000
)

var errImageTooLarge = errors.New("image is too large")

func decodeImage(r io.Reader) (image.Image, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	return decodeImageBytes(data)
}

func decodeImageBytes(data []byte) (image.Image, error) {
	cfg, _, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if cfg.Width <= 0 || cfg.Height <= 0 || cfg.Width > maxImageSide || cfg.Height > maxImageSide || cfg.Width*cfg.Height > maxImagePixels {
		return nil, errImageTooLarge
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}
