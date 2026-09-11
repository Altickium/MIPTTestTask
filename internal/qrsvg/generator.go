package qrsvg

import (
	"fmt"
	"strings"

	qrcode "github.com/skip2/go-qrcode"
)

type Generator struct{}

func (Generator) GenerateSVG(content string) ([]byte, error) {
	code, err := qrcode.New(content, qrcode.Medium)
	if err != nil {
		return nil, err
	}
	bitmap := code.Bitmap()
	if len(bitmap) == 0 {
		return nil, fmt.Errorf("QR encoder returned an empty bitmap")
	}
	var path strings.Builder
	for y, row := range bitmap {
		for x, dark := range row {
			if dark {
				fmt.Fprintf(&path, "M%d %dh1v1h-1z", x, y)
			}
		}
	}
	var svg strings.Builder
	fmt.Fprintf(&svg, `<?xml version="1.0" encoding="UTF-8"?><svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 %d %d" width="512" height="512" role="img" aria-label="QR code" shape-rendering="crispEdges"><rect width="100%%" height="100%%" fill="#fff"/><path d="%s" fill="#111827"/></svg>`, len(bitmap), len(bitmap), path.String())
	return []byte(svg.String()), nil
}
