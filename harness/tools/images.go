package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"harness/harness/appdir"
	"harness/harness/events"
)

// imageResult is one ready-to-use photo the model can drop into an <img src>.
type imageResult struct {
	URL      string `json:"url"`
	Alt      string `json:"alt"`
	Width    int    `json:"width,omitempty"`
	Height   int    `json:"height,omitempty"`
	AvgColor string `json:"avg_color,omitempty"`
	Credit   string `json:"credit,omitempty"`
}

// searchImagesTool returns real, hotlinkable photo URLs for a query so a small
// model never has to invent image URLs (which 404) or fake an image with an
// empty coloured box. With PEXELS_API_KEY set it uses the Pexels photo API
// (relevant, curated results); without a key it falls back to Lorem Picsum (real
// photos, seeded by the query so they are stable, no key required).
func searchImagesTool() *Tool {
	return &Tool{
		Name:        "search_images",
		Description: "Find real, ready-to-use photo URLs for the app (hero image, product/card thumbnails, avatars, backgrounds, gallery). Returns {url, alt, width, height, avg_color}. Put a returned url straight into <img src=\"...\">. NEVER invent image URLs yourself and NEVER fake an image with an empty coloured/gradient box — use this tool whenever the design needs a real photo. Query in plain words describing the subject, e.g. \"video game controller\", \"cheeseburger on a plate\", \"mountain landscape at sunset\".",
		Params:      json.RawMessage(`{"type":"object","properties":{"query":{"type":"string","description":"What the images should show, in plain words."},"count":{"type":"integer","description":"How many images to return (1-10).","default":4},"orientation":{"type":"string","enum":["landscape","portrait","square"],"description":"Preferred shape of the images. Optional."}},"required":["query"]}`),
		WebSafe:     true,
		Run: func(env Env, args Args) (any, *events.Diff, error) {
			query := strings.TrimSpace(args.Str("query"))
			if query == "" {
				return nil, nil, fmt.Errorf("query is required")
			}
			count := args.Int("count", 4)
			if count < 1 {
				count = 1
			}
			if count > 10 {
				count = 10
			}
			orientation := args.Str("orientation")

			source := "pexels"
			var images []imageResult
			if key := pexelsKey(); key != "" {
				if got, err := pexelsSearch(env.Ctx, query, count, orientation, key); err == nil {
					images = got
				}
			}
			// No key, an API error, or an empty result: fall back to seeded real
			// photos so the model always gets usable URLs.
			if len(images) == 0 {
				images = picsumImages(query, count, orientation)
				source = "picsum"
			}
			return map[string]any{
				"query":  query,
				"source": source,
				"images": images,
				"note":   "Use these url values directly in <img src>. Set object-cover and a fixed aspect ratio or width/height so the layout stays stable.",
			}, nil, nil
		},
	}
}

// pexelsKey resolves the Pexels API key from PEXELS_API_KEY, then from a
// "pexels_key" file in the local-pilot config dir (~/.localpilot/pexels_key), so
// the user can enable relevant, subject-matched images without an env var.
func pexelsKey() string {
	if k := strings.TrimSpace(os.Getenv("PEXELS_API_KEY")); k != "" {
		return k
	}
	if b, err := os.ReadFile(filepath.Join(appdir.Dir(), "pexels_key")); err == nil {
		return strings.TrimSpace(string(b))
	}
	return ""
}

// pexelsSearch queries the Pexels photo search API (requires an API key).
func pexelsSearch(ctx context.Context, query string, count int, orientation, key string) ([]imageResult, error) {
	endpoint := fmt.Sprintf("https://api.pexels.com/v1/search?per_page=%d&query=%s", count, url.QueryEscape(query))
	if orientation == "landscape" || orientation == "portrait" || orientation == "square" {
		endpoint += "&orientation=" + orientation
	}
	var body struct {
		Photos []struct {
			Width        int    `json:"width"`
			Height       int    `json:"height"`
			AvgColor     string `json:"avg_color"`
			Photographer string `json:"photographer"`
			Alt          string `json:"alt"`
			Src          struct {
				Large     string `json:"large"`
				Large2x   string `json:"large2x"`
				Medium    string `json:"medium"`
				Landscape string `json:"landscape"`
				Portrait  string `json:"portrait"`
			} `json:"src"`
		} `json:"photos"`
	}
	headers := map[string]string{"Authorization": key}
	if err := httpGetJSON(ctx, endpoint, headers, &body); err != nil {
		return nil, err
	}
	var out []imageResult
	for _, p := range body.Photos {
		src := p.Src.Large
		switch orientation {
		case "landscape":
			if p.Src.Landscape != "" {
				src = p.Src.Landscape
			}
		case "portrait":
			if p.Src.Portrait != "" {
				src = p.Src.Portrait
			}
		}
		if src == "" {
			continue
		}
		alt := p.Alt
		if alt == "" {
			alt = query
		}
		out = append(out, imageResult{
			URL:      src,
			Alt:      alt,
			Width:    p.Width,
			Height:   p.Height,
			AvgColor: p.AvgColor,
			Credit:   p.Photographer,
		})
	}
	return out, nil
}

// picsumImages builds seeded Lorem Picsum URLs — real photographs, no API key.
// Seeding by the query (plus an index) keeps the same query stable across builds
// while still returning several distinct images.
func picsumImages(query string, count int, orientation string) []imageResult {
	w, h := 1200, 800
	switch orientation {
	case "portrait":
		w, h = 800, 1200
	case "square":
		w, h = 900, 900
	}
	base := imageSlug(query)
	out := make([]imageResult, 0, count)
	for i := 0; i < count; i++ {
		seed := fmt.Sprintf("%s-%d", base, i+1)
		out = append(out, imageResult{
			URL:    fmt.Sprintf("https://picsum.photos/seed/%s/%d/%d", url.PathEscape(seed), w, h),
			Alt:    query,
			Width:  w,
			Height: h,
		})
	}
	return out
}

// imageSlug turns a free-text query into a stable, URL-safe seed.
func imageSlug(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	var b strings.Builder
	prevDash := false
	for _, r := range s {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	slug := strings.Trim(b.String(), "-")
	if slug == "" {
		slug = "photo"
	}
	return slug
}
