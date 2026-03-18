package exporter

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type ProcessResult struct {
	Name  string
	PhSeq []string
	PhDur []float64
	Error error
}

func (r *ProcessResult) String() string {
	if r.Error != nil {
		return fmt.Sprintf("ProcessResult{Name: %q, Error: %v}", r.Name, r.Error)
	}
	return fmt.Sprintf("ProcessResult{Name: %q, PhSeq: %v}", r.Name, r.PhSeq)
}

type DatasetBuilder struct {
	wavProcessor *WAVProcessor
	tgParser     *TextGridParser
	csvGenerator *CSVGenerator
}

func NewDatasetBuilder() *DatasetBuilder {
	return &DatasetBuilder{
		wavProcessor: NewWAVProcessor(),
		tgParser:     NewTextGridParser(),
		csvGenerator: NewCSVGenerator(),
	}
}

func (b *DatasetBuilder) ProcessFile(wavPath, tgPath, outputPath string, cfg *Config) (*ProcessResult, error) {
	result := &ProcessResult{
		Name: strings.TrimSuffix(filepath.Base(wavPath), filepath.Ext(wavPath)),
	}
	tg, err := b.tgParser.Parse(tgPath)
	if err != nil {
		result.Error = err
		return result, err
	}
	phonemeData, err := b.tgParser.ExtractPhonemes(tg, cfg.PhoneTierName)
	if err != nil {
		result.Error = err
		return result, err
	}
	result.PhSeq = phonemeData.PhSeq
	result.PhDur = phonemeData.PhDur
	if err := b.wavProcessor.CopyOrConvertWAV(wavPath, outputPath, cfg.TargetSampleRate, cfg.WavSubtype); err != nil {
		result.Error = err
		return result, err
	}
	return result, nil
}

type buildJob struct {
	wavPath    string
	tgPath     string
	outputPath string
}

type buildJobResult struct {
	record *DatasetRecord
	errMsg string
}

type BuildResult struct {
	TotalFiles   int
	SuccessCount int
	ErrorCount   int
	Records      []*DatasetRecord
	Errors       []string
}

func (r *BuildResult) String() string {
	return fmt.Sprintf("BuildResult{TotalFiles: %d, SuccessCount: %d, ErrorCount: %d}",
		r.TotalFiles, r.SuccessCount, r.ErrorCount)
}

func (b *DatasetBuilder) Build(wavsDir, tgDir, outputDir string, cfg *Config) (*BuildResult, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	wavsOutDir := filepath.Join(outputDir, "wavs")
	if err := os.MkdirAll(wavsOutDir, 0755); err != nil {
		return nil, NewFileError(fmt.Sprintf("无法创建 wavs 目录: %s", wavsOutDir), err)
	}

	entries, err := os.ReadDir(wavsDir)
	if err != nil {
		return nil, NewFileError(fmt.Sprintf("无法读取WAV目录: %s", wavsDir), err)
	}

	result := &BuildResult{Records: make([]*DatasetRecord, 0), Errors: make([]string, 0)}
	var jobs []buildJob
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if strings.ToLower(filepath.Ext(entry.Name())) != ".wav" {
			continue
		}
		baseName := strings.TrimSuffix(entry.Name(), filepath.Ext(entry.Name()))
		tgPath := filepath.Join(tgDir, baseName+".TextGrid")
		if _, err := os.Stat(tgPath); os.IsNotExist(err) {
			result.Errors = append(result.Errors, fmt.Sprintf("缺少TextGrid文件: %s", tgPath))
			result.ErrorCount++
			continue
		}
		jobs = append(jobs, buildJob{
			wavPath:    filepath.Join(wavsDir, entry.Name()),
			tgPath:     tgPath,
			outputPath: filepath.Join(wavsOutDir, entry.Name()),
		})
	}

	if len(jobs) == 0 {
		result.TotalFiles = result.ErrorCount
		return result, nil
	}

	concurrency := runtime.NumCPU()
	if concurrency > len(jobs) {
		concurrency = len(jobs)
	}

	jobCh := make(chan buildJob, len(jobs))
	for _, j := range jobs {
		jobCh <- j
	}
	close(jobCh)

	resCh := make(chan buildJobResult, len(jobs))
	var wg sync.WaitGroup
	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			// 每个 goroutine 用独立的 builder 避免竞争
			lb := &DatasetBuilder{
				wavProcessor: NewWAVProcessor(),
				tgParser:     NewTextGridParser(),
				csvGenerator: NewCSVGenerator(),
			}
			for job := range jobCh {
				pr, err := lb.ProcessFile(job.wavPath, job.tgPath, job.outputPath, cfg)
				if err != nil {
					resCh <- buildJobResult{errMsg: fmt.Sprintf("处理失败 %s: %v", filepath.Base(job.wavPath), err)}
				} else {
					resCh <- buildJobResult{record: &DatasetRecord{
						Name:  pr.Name,
						PhSeq: pr.PhSeq,
						PhDur: pr.PhDur,
					}}
				}
			}
		}()
	}
	wg.Wait()
	close(resCh)

	for r := range resCh {
		if r.errMsg != "" {
			result.Errors = append(result.Errors, r.errMsg)
			result.ErrorCount++
		} else {
			result.Records = append(result.Records, r.record)
			result.SuccessCount++
		}
	}
	result.TotalFiles = result.SuccessCount + result.ErrorCount
	return result, nil
}

func (b *DatasetBuilder) Export(wavsDir, tgDir, outputDir string, cfg *Config) (*BuildResult, error) {
	result, err := b.Build(wavsDir, tgDir, outputDir, cfg)
	if err != nil {
		return nil, err
	}
	csvPath := filepath.Join(outputDir, "transcriptions.csv")
	if err := b.csvGenerator.Generate(result.Records, csvPath); err != nil {
		return nil, err
	}
	return result, nil
}
