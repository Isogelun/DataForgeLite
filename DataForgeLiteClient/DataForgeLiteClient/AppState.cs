using System;
using System.Collections.ObjectModel;
using System.IO;
using System.Linq;

namespace DataForgeLiteClient
{
    /// <summary>单条音频文件项（用于列表展示）</summary>
    public class AudioFileItem
    {
        public string FilePath { get; set; }
        public string FileName { get; set; }
        public string Format { get; set; }
        public string Duration { get; set; }
        public string SampleRate { get; set; }
        public string Status { get; set; }
        public string Note { get; set; }
    }

    /// <summary>切片项（预处理/切分 Tab）</summary>
    public class SliceItem
    {
        public string SliceId { get; set; }
        public string Start { get; set; }
        public string End { get; set; }
        public string Duration { get; set; }
        public string Status { get; set; }
    }

    /// <summary>ASR 识别结果项</summary>
    public class ASRResultItem
    {
        public string SliceId { get; set; }
        public string RawText { get; set; }
        public string ProcessedText { get; set; }
        /// <summary>无标点、无声调拼音（来自 _pinyin.txt）</summary>
        public string Pinyin { get; set; }
        public string Language { get; set; }
        public string Confidence { get; set; }
        public string Status { get; set; }
        public string FilePath { get; set; }
    }

    /// <summary>人工校正项（与 ASR 结果对应，可编辑文本）</summary>
    public class CorrectionItem
    {
        public string DisplayName { get; set; }
        public string TimeRange { get; set; }
        public string Status { get; set; }
        public string Text { get; set; }
        /// <summary>无标点、无声调拼音（与 .lab / _pinyin.txt 一致）</summary>
        public string Pinyin { get; set; }
        public string WavPath { get; set; }
        public string TxtPath { get; set; }
    }

    /// <summary>导出任务项</summary>
    public class ExportItem
    {
        public string Name { get; set; }
        public string AlignStatus { get; set; }
        public string TextGridPath { get; set; }
        public string Format { get; set; }
        public string OutputDir { get; set; }
    }

    /// <summary>项目状态与全局数据，供各 Tab 绑定</summary>
    public class AppState
    {
        public static readonly AppState Instance = new AppState();

        public string ProjectDir { get; set; }
        /// <summary>导入音频所在目录 = 项目根目录，不单独建 raw 子目录</summary>
        public string RawDir { get { return ProjectDir; } }
        public string PreprocessedDir { get { return string.IsNullOrEmpty(ProjectDir) ? null : Path.Combine(ProjectDir, "preprocessed"); } }
        public string SlicesDir { get { return string.IsNullOrEmpty(ProjectDir) ? null : Path.Combine(ProjectDir, "slices"); } }
        public string AsrOutputDir { get { return string.IsNullOrEmpty(ProjectDir) ? null : Path.Combine(ProjectDir, "asr_output"); } }

        public ObservableCollection<AudioFileItem> AudioFiles { get; } = new ObservableCollection<AudioFileItem>();
        public ObservableCollection<SliceItem> Slices { get; } = new ObservableCollection<SliceItem>();
        public ObservableCollection<ASRResultItem> ASRResults { get; } = new ObservableCollection<ASRResultItem>();
        public ObservableCollection<CorrectionItem> CorrectionList { get; } = new ObservableCollection<CorrectionItem>();
        public ObservableCollection<string> Logs { get; } = new ObservableCollection<string>();
        public ObservableCollection<ExportItem> ExportList { get; } = new ObservableCollection<ExportItem>();

        public int AudioFileCount { get { return AudioFiles.Count; } }
        public int SliceCount { get { return Slices.Count; } }
        public string TotalDurationDisplay { get; set; }
        public int ProgressPercent { get; set; }
        public string ProgressText { get; set; }

        private AppState()
        {
            ProgressText = "请选择文件夹导入音频或直接填写路径";
        }

        public void AddLog(string message)
        {
            var line = "[" + DateTime.Now.ToString("HH:mm:ss") + "] " + message;
            Logs.Insert(0, line);
            if (Logs.Count > 200) Logs.RemoveAt(Logs.Count - 1);
        }

        public void ClearProject()
        {
            ProjectDir = null;
            AudioFiles.Clear();
            Slices.Clear();
            ASRResults.Clear();
            CorrectionList.Clear();
            ExportList.Clear();
            Logs.Clear();
            TotalDurationDisplay = "00:00:00";
            ProgressPercent = 0;
            ProgressText = "请选择文件夹导入音频或直接填写路径";
        }

        public void EnsureProjectDirs()
        {
            if (string.IsNullOrEmpty(ProjectDir)) return;
            // 只在项目目录下创建预处理、切分、ASR 输出子目录；音频直接在项目根目录
            foreach (var d in new[] { PreprocessedDir, SlicesDir, AsrOutputDir })
            {
                if (!string.IsNullOrEmpty(d)) Directory.CreateDirectory(d);
            }
        }
    }
}
