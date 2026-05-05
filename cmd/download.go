package cmd

import (
	"bilinovel-downloader/downloader"
	"bilinovel-downloader/downloader/bilinovel"
	"bilinovel-downloader/epub"
	"bilinovel-downloader/model"
	"bilinovel-downloader/text"
	"bilinovel-downloader/utils"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
)

var downloadCmd = &cobra.Command{
	Use:   "download",
	Short: "Download a novel or volume",
	Long:  "Download a novel or volume",
	Run: func(cmd *cobra.Command, args []string) {
		slog.Info("Downloading novel")

		err := runDownloadNovel()
		if err != nil {
			slog.Error("failed to download novel", slog.Any("error", err))
			return
		}
	},
}

type downloadCmdArgs struct {
	NovelId     int `validate:"required"`
	VolumeId    int `validate:"required"`
	outputPath  string
	auxPath     string
	outputType  string
	concurrency int
	debug       bool
	cleanAux    bool
}

type downloadOptions struct {
	Context              context.Context
	NovelId              int
	VolumeId             int
	OutputPath           string
	AuxPath              string
	OutputType           string
	Concurrency          int
	Debug                bool
	OrganizeByNovelTitle bool
	CleanAux             bool
}

var (
	downloadArgs downloadCmdArgs
)

func init() {
	downloadCmd.Flags().IntVarP(&downloadArgs.NovelId, "novel-id", "n", 0, "novel id")
	downloadCmd.Flags().IntVarP(&downloadArgs.VolumeId, "volume-id", "v", 0, "volume id")
	downloadCmd.Flags().StringVarP(&downloadArgs.outputPath, "output-path", "o", "novels", "output path")
	downloadCmd.Flags().StringVar(&downloadArgs.auxPath, "aux-path", "", "auxiliary file path for json caches and epub staging; defaults to output path")
	downloadCmd.Flags().StringVarP(&downloadArgs.outputType, "output-type", "t", "epub", "output type, epub or text")
	downloadCmd.Flags().BoolVar(&downloadArgs.debug, "debug", false, "debug mode")
	downloadCmd.Flags().IntVar(&downloadArgs.concurrency, "concurrency", 3, "concurrency of downloading volumes")
	downloadCmd.Flags().BoolVar(&downloadArgs.cleanAux, "clean-aux", false, "remove auxiliary json and epub staging files after successful generation")
	RootCmd.AddCommand(downloadCmd)
}

func runDownloadNovel() error {
	return runDownload(downloadOptions{
		NovelId:     downloadArgs.NovelId,
		VolumeId:    downloadArgs.VolumeId,
		OutputPath:  downloadArgs.outputPath,
		AuxPath:     downloadArgs.auxPath,
		OutputType:  downloadArgs.outputType,
		Concurrency: downloadArgs.concurrency,
		Debug:       downloadArgs.debug,
		CleanAux:    downloadArgs.cleanAux,
	})
}

func runDownload(options downloadOptions) error {
	if options.Context == nil {
		options.Context = context.Background()
	}
	if err := checkDownloadContext(options.Context); err != nil {
		return err
	}

	downloader, err := bilinovel.New(bilinovel.BilinovelNewOption{
		Concurrency: options.Concurrency,
		Debug:       options.Debug,
	})
	if err != nil {
		return fmt.Errorf("failed to create downloader: %v", err)
	}
	// 确保在函数结束时关闭资源
	defer func() {
		if closeErr := downloader.Close(); closeErr != nil {
			slog.Info("Failed to close downloader", slog.Any("error", closeErr))
		}
	}()

	if options.NovelId == 0 {
		return fmt.Errorf("novel id is required")
	}
	if options.OutputPath == "" {
		return fmt.Errorf("output path is required")
	}
	if options.AuxPath == "" {
		options.AuxPath = options.OutputPath
	}
	if options.OutputType == "" {
		options.OutputType = "epub"
	}
	if err := checkDownloadContext(options.Context); err != nil {
		return err
	}

	if options.VolumeId == 0 {
		// 下载整本小说
		err := downloadNovel(downloader, options)
		if err != nil {
			return fmt.Errorf("failed to get novel: %v", err)
		}
	} else {
		// 下载单卷
		err = downloadVolume(downloader, options)
		if err != nil {
			return fmt.Errorf("failed to download volume: %v", err)
		}
	}

	return nil
}

func downloadNovel(downloader downloader.Downloader, options downloadOptions) error {
	if err := checkDownloadContext(options.Context); err != nil {
		return err
	}
	novelInfo, err := downloader.GetNovel(options.NovelId, true, nil)
	if err != nil {
		return fmt.Errorf("failed to get novel info: %w", err)
	}
	if err := checkDownloadContext(options.Context); err != nil {
		return err
	}
	outputPath := resolveNovelOutputPath(options.OutputPath, novelInfo.Title, options.OrganizeByNovelTitle)
	auxPath := resolveNovelOutputPath(options.AuxPath, novelInfo.Title, options.OrganizeByNovelTitle)
	skipVolumes := make([]int, 0)
	for _, volume := range novelInfo.Volumes {
		jsonPath := volumeJSONPath(auxPath, options.NovelId, volume.Id)
		err = os.MkdirAll(filepath.Dir(jsonPath), 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
		_, err = os.Stat(jsonPath)
		if err == nil {
			// 已经下载
			skipVolumes = append(skipVolumes, volume.Id)
		}
	}
	if err := checkDownloadContext(options.Context); err != nil {
		return err
	}
	novel, err := downloader.GetNovel(options.NovelId, false, skipVolumes)
	if err != nil {
		return fmt.Errorf("failed to download novel: %w", err)
	}
	for _, volume := range novel.Volumes {
		if err := checkDownloadContext(options.Context); err != nil {
			return err
		}
		jsonPath := volumeJSONPath(auxPath, options.NovelId, volume.Id)
		err = os.MkdirAll(filepath.Dir(jsonPath), 0755)
		if err != nil {
			return fmt.Errorf("failed to create directory: %v", err)
		}
		jsonFile, err := os.Create(jsonPath)
		if err != nil {
			return fmt.Errorf("failed to create json file: %v", err)
		}
		err = json.NewEncoder(jsonFile).Encode(volume)
		if err != nil {
			_ = jsonFile.Close()
			return fmt.Errorf("failed to encode json file: %v", err)
		}
		err = jsonFile.Close()
		if err != nil {
			return fmt.Errorf("failed to close json file: %v", err)
		}
		switch options.OutputType {
		case "epub":
			err = epub.PackVolumeToEpubWithOptions(volume, epub.PackVolumeOptions{
				OutputPath: outputPath,
				AuxPath:    auxPath,
				CleanAux:   options.CleanAux,
				StyleCSS:   downloader.GetStyleCSS(),
				ExtraFiles: downloader.GetExtraFiles(),
			})
			if err != nil {
				return fmt.Errorf("failed to pack volume: %v", err)
			}
			if options.CleanAux {
				err = os.Remove(jsonPath)
				if err != nil && !os.IsNotExist(err) {
					return fmt.Errorf("failed to remove json file: %v", err)
				}
			}
		case "text":
			err = text.PackVolumeToText(volume, outputPath)
			if err != nil {
				return fmt.Errorf("failed to pack volume: %v", err)
			}
		default:
			return fmt.Errorf("unsupported output type: %s", options.OutputType)
		}
	}
	return nil
}

func downloadVolume(downloader downloader.Downloader, options downloadOptions) error {
	if err := checkDownloadContext(options.Context); err != nil {
		return err
	}
	outputPath := options.OutputPath
	auxPath := options.AuxPath
	if options.OrganizeByNovelTitle {
		volume, err := downloader.GetVolume(options.NovelId, options.VolumeId, false)
		if err != nil {
			return fmt.Errorf("failed to get volume: %v", err)
		}
		if err := checkDownloadContext(options.Context); err != nil {
			return err
		}
		outputPath = resolveNovelOutputPath(options.OutputPath, volume.NovelTitle, true)
		auxPath = resolveNovelOutputPath(options.AuxPath, volume.NovelTitle, true)
		return saveVolume(downloader, volume, outputPath, auxPath, options)
	}

	jsonPath := volumeJSONPath(auxPath, options.NovelId, options.VolumeId)
	err := os.MkdirAll(filepath.Dir(jsonPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}
	_, err = os.Stat(jsonPath)
	volume := &model.Volume{}
	if err != nil {
		if os.IsNotExist(err) {
			volume, err = downloader.GetVolume(options.NovelId, options.VolumeId, false)
			if err != nil {
				return fmt.Errorf("failed to get volume: %v", err)
			}
			jsonFile, err := os.Create(jsonPath)
			if err != nil {
				return fmt.Errorf("failed to create json file: %v", err)
			}
			err = json.NewEncoder(jsonFile).Encode(volume)
			if err != nil {
				_ = jsonFile.Close()
				return fmt.Errorf("failed to encode json file: %v", err)
			}
			err = jsonFile.Close()
			if err != nil {
				return fmt.Errorf("failed to close json file: %v", err)
			}
		} else {
			return fmt.Errorf("failed to get volume: %v", err)
		}
	} else {
		jsonFile, err := os.Open(jsonPath)
		if err != nil {
			return fmt.Errorf("failed to open json file: %v", err)
		}
		defer jsonFile.Close()
		err = json.NewDecoder(jsonFile).Decode(volume)
		if err != nil {
			return fmt.Errorf("failed to decode json file: %v", err)
		}
	}

	return packVolume(downloader, volume, outputPath, auxPath, options)
}

func saveVolume(downloader downloader.Downloader, volume *model.Volume, outputPath string, auxPath string, options downloadOptions) error {
	if err := checkDownloadContext(options.Context); err != nil {
		return err
	}
	jsonPath := volumeJSONPath(auxPath, options.NovelId, volume.Id)
	err := os.MkdirAll(filepath.Dir(jsonPath), 0755)
	if err != nil {
		return fmt.Errorf("failed to create directory: %v", err)
	}
	jsonFile, err := os.Create(jsonPath)
	if err != nil {
		return fmt.Errorf("failed to create json file: %v", err)
	}
	err = json.NewEncoder(jsonFile).Encode(volume)
	if err != nil {
		_ = jsonFile.Close()
		return fmt.Errorf("failed to encode json file: %v", err)
	}
	err = jsonFile.Close()
	if err != nil {
		return fmt.Errorf("failed to close json file: %v", err)
	}

	return packVolume(downloader, volume, outputPath, auxPath, options)
}

func packVolume(downloader downloader.Downloader, volume *model.Volume, outputPath string, auxPath string, options downloadOptions) error {
	if err := checkDownloadContext(options.Context); err != nil {
		return err
	}
	switch options.OutputType {
	case "epub":
		err := epub.PackVolumeToEpubWithOptions(volume, epub.PackVolumeOptions{
			OutputPath: outputPath,
			AuxPath:    auxPath,
			CleanAux:   options.CleanAux,
			StyleCSS:   downloader.GetStyleCSS(),
			ExtraFiles: downloader.GetExtraFiles(),
		})
		if err != nil {
			return fmt.Errorf("failed to pack volume: %v", err)
		}
		if options.CleanAux {
			jsonPath := volumeJSONPath(auxPath, options.NovelId, volume.Id)
			err = os.Remove(jsonPath)
			if err != nil && !os.IsNotExist(err) {
				return fmt.Errorf("failed to remove json file: %v", err)
			}
		}
	case "text":
		err := text.PackVolumeToText(volume, outputPath)
		if err != nil {
			return fmt.Errorf("failed to pack volume: %v", err)
		}
	default:
		return fmt.Errorf("unsupported output type: %s", options.OutputType)
	}
	return nil
}

func volumeJSONPath(auxPath string, novelID int, volumeID int) string {
	return filepath.Join(auxPath, fmt.Sprintf("volume-%d-%d.json", novelID, volumeID))
}

func checkDownloadContext(ctx context.Context) error {
	if ctx == nil {
		return nil
	}
	if err := ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return fmt.Errorf("download canceled: %w", err)
		}
		return err
	}
	return nil
}

func resolveNovelOutputPath(outputPath string, novelTitle string, organizeByNovelTitle bool) string {
	if !organizeByNovelTitle || novelTitle == "" {
		return outputPath
	}
	return filepath.Join(outputPath, utils.CleanDirName(novelTitle))
}
