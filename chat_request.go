package main

import (
	"encoding/base64"
	"errors"
	"io"
	"net/http"
	"strings"
)

const defaultMaxImageBytes int64 = 10 << 20
const defaultMaxChatRequestBytes int64 = 32 << 20

var errEmptyChatRequest = errors.New("message or image required")
var errUnsupportedImageType = errors.New("only image attachments are supported")
var errImageTooLarge = errors.New("image attachment too large")

type ChatImage struct {
	Type     string `json:"type"`
	Data     string `json:"data"`
	MimeType string `json:"mimeType"`
}

type ChatRequest struct {
	Message string
	Images  []ChatImage
}

func parseChatRequest(r *http.Request, maxImageBytes, maxRequestBytes int64) (ChatRequest, error) {
	r.Body = http.MaxBytesReader(nil, r.Body, maxRequestBytes)
	if err := r.ParseMultipartForm(maxRequestBytes); err != nil {
		return ChatRequest{}, err
	}

	chat := ChatRequest{Message: strings.TrimSpace(r.FormValue("message"))}
	if r.MultipartForm == nil {
		if chat.Message == "" {
			return ChatRequest{}, errEmptyChatRequest
		}
		return chat, nil
	}

	for _, fh := range r.MultipartForm.File["images"] {
		file, err := fh.Open()
		if err != nil {
			return ChatRequest{}, err
		}
		data, readErr := io.ReadAll(io.LimitReader(file, maxImageBytes+1))
		closeErr := file.Close()
		if readErr != nil {
			return ChatRequest{}, readErr
		}
		if closeErr != nil {
			return ChatRequest{}, closeErr
		}
		if int64(len(data)) > maxImageBytes {
			return ChatRequest{}, errImageTooLarge
		}
		mimeType := http.DetectContentType(data)
		if !strings.HasPrefix(mimeType, "image/") {
			return ChatRequest{}, errUnsupportedImageType
		}
		chat.Images = append(chat.Images, ChatImage{Type: "image", Data: base64.StdEncoding.EncodeToString(data), MimeType: mimeType})
	}

	if chat.Message == "" && len(chat.Images) == 0 {
		return ChatRequest{}, errEmptyChatRequest
	}
	return chat, nil
}
