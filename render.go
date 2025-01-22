package live

import (
	"bytes"
	"context"
	"fmt"
	"html/template"
	"io"
	"log"
	"os"
	"runtime/debug"
	"sync"

	"golang.org/x/net/html"
)

// RenderContext contains the sockets current data for rendering.
type RenderContext struct {
	Socket  Socket
	Uploads UploadContext
	Assigns interface{}
}

var _counters = map[SocketID]int{}
var _countersMu sync.Mutex

// RenderSocket takes the engine and current socket and renders it to html.
func RenderSocket(ctx context.Context, e Engine, s Socket) (*html.Node, error) {
	rc := &RenderContext{
		Socket:  s,
		Uploads: s.Uploads(),
		Assigns: s.Assigns(),
	}

	output, err := e.Render()(ctx, rc)
	if err != nil {
		return nil, fmt.Errorf("render error: %w", err)
	}
	render, err := html.Parse(output)
	if err != nil {
		return nil, fmt.Errorf("html parse error: %w", err)
	}
	shapeTree(render)

	if s.LatestRender() != nil {
		patches, err := Diff(s.LatestRender(), render)
		if err != nil {
			return nil, fmt.Errorf("diff error: %w", err)
		}
		if len(patches) != 0 {
			err := s.Send(EventPatch, patches)
			if err != nil {
				return nil, fmt.Errorf("send error: %w", err)
			}

			// TEMP: Save the current and proposed renders to files.
			_countersMu.Lock()
			counter, ok := _counters[s.ID()]
			if !ok {
				counter = 0
			}
			_counters[s.ID()] = counter + 1
			_countersMu.Unlock()

			log.Println(string(debug.Stack()))

			fc, err := os.OpenFile(fmt.Sprintf("%s-%d-current.html", s.ID(), counter), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				defer fc.Close()

				var d bytes.Buffer
				html.Render(&d, s.LatestRender())

				fc.WriteString(d.String())
			}

			fp, err := os.OpenFile(fmt.Sprintf("%s-%d-proposed.html", s.ID(), counter), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				defer fp.Close()

				var d bytes.Buffer
				html.Render(&d, render)

				fp.WriteString(d.String())
			}

			fr, err := os.OpenFile(fmt.Sprintf("%s-%d-results.html", s.ID(), counter), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err == nil {
				defer fr.Close()

				for _, p := range patches {
					fr.WriteString(p.String() + "\n")
				}
			}
		}
	} else {
		anchorTree(render, newAnchorGenerator())
	}

	return render, nil
}

// WithTemplateRenderer set the handler to use an `html/template` renderer.
func WithTemplateRenderer(t *template.Template) HandlerConfig {
	return func(h Handler) error {
		h.HandleRender(func(ctx context.Context, rc *RenderContext) (io.Reader, error) {
			var buf bytes.Buffer
			if err := t.Execute(&buf, rc); err != nil {
				return nil, err
			}
			return &buf, nil
		})
		return nil
	}
}
