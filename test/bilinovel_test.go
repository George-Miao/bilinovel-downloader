package test

import (
	bilinovelDownloader "bilinovel-downloader/downloader/bilinovel"
	"encoding/json"
	"fmt"
	"os"
	"testing"
)

func TestBilinovel_GetNovel(t *testing.T) {
	bilinovel := newIntegrationBilinovel(t, 5)
	novel, err := bilinovel.GetNovel(2727, false, nil)
	if err != nil {
		t.Fatalf("failed to get novel: %v", err)
	}
	jsonBytes, err := json.Marshal(novel)
	if err != nil {
		t.Fatalf("failed to marshal novel: %v", err)
	}
	fmt.Println(string(jsonBytes))
}

func TestBilinovel_GetVolume(t *testing.T) {
	bilinovel := newIntegrationBilinovel(t, 1)
	volume, err := bilinovel.GetVolume(2727, 129092, false)
	if err != nil {
		t.Fatalf("failed to get volume: %v", err)
	}
	jsonBytes, err := json.Marshal(volume)
	if err != nil {
		t.Fatalf("failed to marshal volume: %v", err)
	}
	fmt.Println(string(jsonBytes))
}

func TestBilinovel_GetChapter(t *testing.T) {
	bilinovel := newIntegrationBilinovel(t, 1)
	chapter, err := bilinovel.GetChapter(2727, 129092, 129094)
	if err != nil {
		t.Fatalf("failed to get chapter: %v", err)
	}
	jsonBytes, err := json.Marshal(chapter)
	if err != nil {
		t.Fatalf("failed to marshal chapter: %v", err)
	}
	fmt.Println(string(jsonBytes))
}

func newIntegrationBilinovel(t *testing.T, concurrency int) *bilinovelDownloader.Bilinovel {
	t.Helper()
	if os.Getenv("BILINOVEL_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set BILINOVEL_RUN_INTEGRATION_TESTS=1 to run live Bilinovel integration tests")
	}

	bilinovel, err := bilinovelDownloader.New(bilinovelDownloader.BilinovelNewOption{Concurrency: concurrency})
	if err != nil {
		t.Fatalf("failed to create bilinovel: %v", err)
	}
	t.Cleanup(func() {
		_ = bilinovel.Close()
	})
	bilinovel.SetTextOnly(true)
	return bilinovel
}
