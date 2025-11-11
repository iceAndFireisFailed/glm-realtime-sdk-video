package tools

import (
	"fmt"
	"testing"
)

func TestExtractFramesToBase64(t *testing.T) {
	video := ""
	frames, err := ExtractFramesToBase64(video, "Z0LADJoFAAABMA==", "aM48gA==")
	if err != nil {
		panic(err)
	}
	for _, frame := range frames {
		fmt.Println(frame)
	}
}
