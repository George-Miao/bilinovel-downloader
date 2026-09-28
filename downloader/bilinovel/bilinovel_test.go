package bilinovel

import (
	"bilinovel-downloader/model"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/PuerkitoBio/goquery"
	"github.com/mxschmitt/playwright-go"
)

func TestProcessContentWithRelativeScript(t *testing.T) {
	if os.Getenv("BILINOVEL_RUN_INTEGRATION_TESTS") != "1" {
		t.Skip("set BILINOVEL_RUN_INTEGRATION_TESTS=1 to run browser integration tests")
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/themes/zhmb/js/chapterlog.js" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/javascript")
		_, _ = w.Write([]byte(`document.querySelector("#acontent").textContent = "ready";`))
	}))
	defer server.Close()

	if err := playwright.Install(&playwright.RunOptions{SkipInstallBrowsers: true}); err != nil {
		t.Fatal(err)
	}
	pw, err := playwright.Run()
	if err != nil {
		t.Fatal(err)
	}
	defer pw.Stop()

	options := playwright.BrowserTypeLaunchOptions{Headless: playwright.Bool(true)}
	if executable := os.Getenv("PLAYWRIGHT_EXECUTABLE_PATH"); executable != "" {
		options.ExecutablePath = playwright.String(executable)
	}
	browser, err := pw.Chromium.Launch(options)
	if err != nil {
		t.Fatal(err)
	}
	defer browser.Close()
	page, err := browser.NewPage()
	if err != nil {
		t.Fatal(err)
	}

	sourceURL := server.URL + "/novel/2013/165899_1.html"
	content := `<html><head></head><body><div id="acontent">loading</div>` +
		`<script src="/themes/zhmb/js/chapterlog.js"></script></body></html>`
	result, err := (&Bilinovel{logger: slog.Default()}).processContentWithPlaywright(page, content, sourceURL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(result, `<div id="acontent">ready</div>`) {
		t.Fatalf("relative chapter script did not process content: %s (source %s)", result, sourceURL)
	}
}

func TestCleanChapterContentRemovesAdvertisementContainers(t *testing.T) {
	doc, err := goquery.NewDocumentFromReader(strings.NewReader(`
		<div id="acontent">
			<div class="cgo"><ins class="adsbygoogle" style="height: 280px"></ins></div>
			<div class="csgo"><ins class="adsbygoogle" style="height: 280px"></ins></div>
			<p>保留正文</p>
			<ins class="adsbygoogle" style="height: 280px"></ins>
			<div class="google-auto-placed"><iframe></iframe></div>
			<div class="co"><ins class="adsbygoogle" style="height: 280px"></ins></div>
		</div>
	`))
	if err != nil {
		t.Fatalf("failed to parse fixture: %v", err)
	}

	content := doc.Find("#acontent").First()
	cleanChapterContent(content)

	if remaining := content.Find(".cgo, .csgo, .co, .google-auto-placed, ins.adsbygoogle, iframe").Length(); remaining != 0 {
		t.Fatalf("cleaned content still contains %d advertisement elements", remaining)
	}
	html, err := content.Html()
	if err != nil {
		t.Fatalf("failed to render cleaned content: %v", err)
	}
	if !strings.Contains(html, "<p>保留正文</p>") {
		t.Fatalf("cleaned content lost chapter text: %s", html)
	}
}

func TestMergeDownloadedChapterPreservesVolumeTitle(t *testing.T) {
	testCases := []struct {
		name            string
		listedTitle     string
		downloadedTitle string
	}{
		{
			name:            "circle placeholder",
			listedTitle:     "～第三败～ 欲擒主将，可是马儿太强",
			downloadedTitle: "～第三败～ 〇擒主将，可是马儿太强",
		},
		{
			name:            "heart placeholder",
			listedTitle:     "番外篇 真心话",
			downloadedTitle: "番外篇 真♡话",
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			listedChapter := &model.Chapter{
				Title: testCase.listedTitle,
				Url:   "https://www.bilinovel.com/novel/3095/186113.html",
			}
			downloadedChapter := &model.Chapter{
				Id:    186113,
				Title: testCase.downloadedTitle,
				Url:   "https://www.bilinovel.com/novel/3095/186113_1.html",
				Content: &model.ChaperContent{
					Html: "<p>正文</p>",
				},
			}

			mergedChapter := mergeDownloadedChapter(listedChapter, downloadedChapter)

			if mergedChapter.Title != listedChapter.Title {
				t.Fatalf("merged title = %q, want volume title %q", mergedChapter.Title, listedChapter.Title)
			}
			if mergedChapter.Content != downloadedChapter.Content {
				t.Fatal("merged chapter did not retain downloaded content")
			}
			if mergedChapter.Url != listedChapter.Url {
				t.Fatalf("merged URL = %q, want volume URL %q", mergedChapter.Url, listedChapter.Url)
			}
			if mergedChapter.Id != downloadedChapter.Id {
				t.Fatalf("merged chapter id = %d, want downloaded id %d", mergedChapter.Id, downloadedChapter.Id)
			}
		})
	}
}
