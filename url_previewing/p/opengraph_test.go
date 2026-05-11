package p

import "testing"

func TestCalcTitlePrefersTitleTag(t *testing.T) {
	html := `<html><head><title>Example Title</title></head><body><h1>Fallback Heading</h1></body></html>`

	if got := calcTitle(html); got != "Example Title" {
		t.Fatalf("expected title tag to win, got %q", got)
	}
}

func TestCalcDescriptionUsesMetaDescriptionBeforeBodyText(t *testing.T) {
	html := `<html><head><meta name="description" content="Concise summary"></head><body><p>Body text</p></body></html>`

	if got := calcDescription(html); got != "Concise summary" {
		t.Fatalf("expected meta description, got %q", got)
	}
}

func TestCalcImagesReturnsImageWithDimensions(t *testing.T) {
	html := `<html><body><img src="tiny.png" width="5" height="5"><img src="hero.png" width="640" height="480"></body></html>`

	images := calcImages(html)
	if len(images) != 1 {
		t.Fatalf("expected one image, got %d", len(images))
	}
	if images[0].URL != "hero.png" {
		t.Fatalf("expected hero image, got %q", images[0].URL)
	}
}
