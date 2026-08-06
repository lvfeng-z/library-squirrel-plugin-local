package main

import (
	"testing"

	sdkdto "github.com/lvfeng-z/library-squirrel-sdk/dto"
)

func TestClassifyResourceType(t *testing.T) {
	cases := []struct {
		path string
		want string
	}{
		{"photo.jpg", sdkdto.ResourceTypeImage},
		{"photo.JPEG", sdkdto.ResourceTypeImage},
		{"img.png", sdkdto.ResourceTypeImage},
		{"anim.webp", sdkdto.ResourceTypeImage},
		{"anim.gif", sdkdto.ResourceTypeImage},
		{"clip.mp4", sdkdto.ResourceTypeVideo},
		{"clip.MKV", sdkdto.ResourceTypeVideo},
		{"movie.mov", sdkdto.ResourceTypeVideo},
		{"clip.webm", sdkdto.ResourceTypeVideo},
		{"doc.pdf", sdkdto.ResourceTypeDocument},
		{"doc.docx", sdkdto.ResourceTypeDocument},
		{"notes.txt", sdkdto.ResourceTypeDocument},
		{"song.mp3", sdkdto.ResourceTypeAudio},
		{"audio.M4A", sdkdto.ResourceTypeAudio},
		{"track.flac", sdkdto.ResourceTypeAudio},
		{"clip.wav", sdkdto.ResourceTypeAudio},
		{"archive.zip", sdkdto.ResourceTypeUnknown},
		{"noext", sdkdto.ResourceTypeUnknown},
	}
	for _, c := range cases {
		got := classifyResourceType(c.path)
		if got != c.want {
			t.Errorf("classifyResourceType(%q) = %q, want %q", c.path, got, c.want)
		}
	}
}

func TestMainStoreRole(t *testing.T) {
	cases := []struct {
		rt   string
		want string
	}{
		{sdkdto.ResourceTypeImage, sdkdto.StoreRoleImage},
		{sdkdto.ResourceTypeVideo, sdkdto.StoreRoleVideoMain},
		{sdkdto.ResourceTypeAudio, sdkdto.StoreRoleAudioMain},
		{sdkdto.ResourceTypeDocument, sdkdto.StoreRoleDocument},
		{sdkdto.ResourceTypeUnknown, sdkdto.StoreRoleImage},
	}
	for _, c := range cases {
		got := mainStoreRole(c.rt)
		if got != c.want {
			t.Errorf("mainStoreRole(%q) = %q, want %q", c.rt, got, c.want)
		}
	}
}
