package main

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
)

type testUpload struct{ name, body string }

func multipartRequest(t *testing.T, message string, files map[string]testUpload) *http.Request {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if message != "" {
		if err := mw.WriteField("message", message); err != nil {
			t.Fatal(err)
		}
	}
	for field, file := range files {
		part, err := mw.CreateFormFile(field, file.name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte(file.body))
	}
	if err := mw.Close(); err != nil {
		t.Fatal(err)
	}
	req, err := http.NewRequest(http.MethodPost, "/api/chat?id=session.jsonl", &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	return req
}

func TestParseChatRequestAcceptsTextOnly(t *testing.T) {
	chat, err := parseChatRequest(multipartRequest(t, "hello", nil), 1024, 4096)
	if err != nil {
		t.Fatalf("parseChatRequest error: %v", err)
	}
	if chat.Message != "hello" || len(chat.Images) != 0 {
		t.Fatalf("chat = %#v", chat)
	}
}

func TestParseChatRequestAcceptsImage(t *testing.T) {
	chat, err := parseChatRequest(multipartRequest(t, "describe", map[string]testUpload{"images": {"a.png", "\x89PNG\r\n\x1a\nimage"}}), 1024, 4096)
	if err != nil {
		t.Fatalf("parseChatRequest error: %v", err)
	}
	if len(chat.Images) != 1 || chat.Images[0].MimeType != "image/png" {
		t.Fatalf("images = %#v", chat.Images)
	}
}

func TestParseChatRequestRejectsEmpty(t *testing.T) {
	_, err := parseChatRequest(multipartRequest(t, "", nil), 1024, 4096)
	if err != errEmptyChatRequest {
		t.Fatalf("err = %v, want errEmptyChatRequest", err)
	}
}

func TestParseChatRequestRejectsNonImage(t *testing.T) {
	_, err := parseChatRequest(multipartRequest(t, "see file", map[string]testUpload{"images": {"a.txt", "plain text"}}), 1024, 4096)
	if err != errUnsupportedImageType {
		t.Fatalf("err = %v, want errUnsupportedImageType", err)
	}
}

func TestParseChatRequestRejectsOversizedImage(t *testing.T) {
	_, err := parseChatRequest(multipartRequest(t, "big", map[string]testUpload{"images": {"a.png", "\x89PNG\r\n\x1a\n" + strings.Repeat("x", 20)}}), 8, 4096)
	if err != errImageTooLarge {
		t.Fatalf("err = %v, want errImageTooLarge", err)
	}
}
