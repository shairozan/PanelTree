package web

import (
	"bytes"
	"fmt"
	"github.com/shairozan/PanelTree/app"
	"golang.org/x/image/draw"
	"image"
	"image/png"
	"io"
	"net/http"
	"net/url"
)

func (s *Server) media(w http.ResponseWriter, r *http.Request, p Project, action string) {
	switch {
	case action == "artworks" && r.Method == "GET":
		items, e := s.service.Artworks(r.Context(), p.Handle)
		if e != nil {
			failure(w, 400, e)
			return
		}
		reply(w, 200, items)
	case action == "media" && r.Method == "GET":
		var data []byte
		var e error
		if r.URL.Query().Get("source") == "1" {
			data, e = s.service.JobSource(r.Context(), p.Handle, r.URL.Query().Get("job"))
		} else {
			data, e = s.service.Media(r.Context(), p.Handle, r.URL.Query().Get("path"), r.URL.Query().Get("job"))
		}
		if e != nil {
			failure(w, 404, e)
			return
		}
		writePNG(w, data, r.URL.Query().Get("thumbnail") == "1")
	case action == "upload" && r.Method == "POST":
		if r.Header.Get("Content-Type") != "image/png" {
			failure(w, 400, fmt.Errorf("PNG upload required"))
			return
		}
		data, e := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<20))
		if e != nil {
			failure(w, 413, e)
			return
		}
		license, e := url.QueryUnescape(r.Header.Get("X-Artwork-License"))
		if e != nil {
			failure(w, 400, e)
			return
		}
		attribution, e := url.QueryUnescape(r.Header.Get("X-Artwork-Attribution"))
		if e != nil {
			failure(w, 400, e)
			return
		}
		item, e := s.service.ImportArtwork(r.Context(), app.ArtworkRequest{ProjectFile: p.Handle, PNG: data, License: license, Attribution: attribution})
		if e != nil {
			failure(w, 400, e)
			return
		}
		reply(w, 201, item)
	default:
		w.WriteHeader(http.StatusMethodNotAllowed)
	}
}

func writePNG(w http.ResponseWriter, data []byte, thumbnail bool) {
	if thumbnail {
		im, e := png.Decode(bytes.NewReader(data))
		if e != nil {
			failure(w, 400, e)
			return
		}
		b := im.Bounds()
		largest := max(b.Dx(), b.Dy())
		if largest > 256 {
			out := image.NewNRGBA(image.Rect(0, 0, max(1, b.Dx()*256/largest), max(1, b.Dy()*256/largest)))
			draw.ApproxBiLinear.Scale(out, out.Bounds(), im, b, draw.Src, nil)
			var encoded bytes.Buffer
			if e = png.Encode(&encoded, out); e != nil {
				failure(w, 500, e)
				return
			}
			data = encoded.Bytes()
		}
	}
	w.Header().Set("Content-Type", "image/png")
	_, _ = w.Write(data)
}
