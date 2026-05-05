package epub

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"bilinovel-downloader/model"
)

func TestCreateContentOPFUsesOneBasedSeriesIndex(t *testing.T) {
	outputPath := t.TempDir()
	volume := &model.Volume{
		Title:       "Volume 1",
		NovelTitle:  "Novel",
		SeriesIdx:   1,
		Description: "description",
	}

	err := CreateContentOPF(outputPath, "test-uuid", volume, nil)
	if err != nil {
		t.Fatalf("CreateContentOPF failed: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(outputPath, "content.opf"))
	if err != nil {
		t.Fatalf("failed to read content.opf: %v", err)
	}

	if !strings.Contains(string(content), `name="calibre:series_index" content="1"`) {
		t.Fatalf("content.opf did not contain one-based series index: %s", string(content))
	}
}

func TestPackVolumeToEpubWithOptionsSeparatesOutputAndCleansAux(t *testing.T) {
	outputPath := t.TempDir()
	auxPath := t.TempDir()
	volume := &model.Volume{
		Title:       "Volume 1",
		NovelTitle:  "Novel",
		SeriesIdx:   1,
		CoverUrl:    "cover.jpg",
		Cover:       []byte("cover"),
		Description: "description",
		Chapters: []*model.Chapter{
			{
				Title: "Chapter 1",
				Content: &model.ChaperContent{
					Html:   "<p>content</p>",
					Images: map[string][]byte{},
				},
			},
		},
	}

	err := PackVolumeToEpubWithOptions(volume, PackVolumeOptions{
		OutputPath: outputPath,
		AuxPath:    auxPath,
		CleanAux:   true,
		StyleCSS:   "body {}",
	})
	if err != nil {
		t.Fatalf("PackVolumeToEpubWithOptions failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(outputPath, "Volume 1.epub")); err != nil {
		t.Fatalf("expected epub in output path: %v", err)
	}
	if _, err := os.Stat(filepath.Join(auxPath, "Volume 1")); !os.IsNotExist(err) {
		t.Fatalf("expected aux staging directory to be removed, got err=%v", err)
	}
}
