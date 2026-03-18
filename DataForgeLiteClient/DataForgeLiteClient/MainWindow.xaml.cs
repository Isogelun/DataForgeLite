namespace DataForgeLiteClient
{
    using System;
    using System.Collections.ObjectModel;
    using System.Diagnostics;
    using System.IO;
    using System.Linq;
    using System.Threading.Tasks;
    using System.Windows;
    using System.Windows.Controls;
    using System.Windows.Data;

    public partial class MainWindow
    {
        private string _goExePath;

        public MainWindow()
        {
            InitializeComponent();
            _goExePath = DataForgeLiteBackend.GetGoExecutablePath();
            DataContext = AppState.Instance;
            RefreshStats();
            // FA 对齐：自动填写 hfa_model 路径（与 exe 同目录或上级）
            if (string.IsNullOrWhiteSpace(TxtFAModelDir?.Text))
                TxtFAModelDir.Text = GetDefaultHFAModelPath() ?? "";
            // 模型目录变更时重新加载模型能力
            if (TxtFAModelDir != null)
                TxtFAModelDir.LostFocus += (s, ev) => LoadHFAModelCapabilities((TxtFAModelDir.Text ?? "").Trim());
            // 首次加载模型能力
            LoadHFAModelCapabilities((TxtFAModelDir?.Text ?? "").Trim());

            AppState.Instance.AddLog("程序已启动");
        }

        /// <summary>
        /// 解析默认 hfa_model 目录：先 exe 同目录，再上级目录（与 Go 端 ResolveDefaultHFAModelDir 一致）。
        /// </summary>
        private static string GetDefaultHFAModelPath()
        {
            var baseDir = AppDomain.CurrentDomain.BaseDirectory;
            if (string.IsNullOrEmpty(baseDir)) return null;
            var dir = Path.GetFullPath(baseDir);
            var candidate = Path.Combine(dir, "hfa_model");
            if (Directory.Exists(candidate)) return candidate;
            var parent = Directory.GetParent(dir);
            if (parent != null)
            {
                candidate = Path.Combine(parent.FullName, "hfa_model");
                if (Directory.Exists(candidate)) return candidate;
            }
            return Path.Combine(dir, "hfa_model");
        }

        private static string GetDefaultGameModelPath()
        {
            try
            {
                var baseDir = AppDomain.CurrentDomain.BaseDirectory;
                if (string.IsNullOrEmpty(baseDir)) return null;
                var dir = Path.GetFullPath(baseDir);
                // 优先：exe 同目录下的 Gameonnx
                var c1 = Path.Combine(dir, "Gameonnx");
                if (Directory.Exists(c1)) return c1;
                // 次选：上级目录（仓库根）下的 Game\...
                var parent = Directory.GetParent(dir);
                if (parent != null)
                {
                    var c2 = Path.Combine(parent.FullName, "Gameonnx");
                    if (Directory.Exists(c2)) return c2;
                }
                return c1;
            }
            catch { return null; }
        }

        private static string GetDefaultOrtDllPath()
        {
            try
            {
                var baseDir = AppDomain.CurrentDomain.BaseDirectory;
                if (string.IsNullOrEmpty(baseDir)) return null;
                var dir = Path.GetFullPath(baseDir);
                var c1 = Path.Combine(dir, "onnxruntime.dll");
                if (File.Exists(c1)) return c1;
                var parent = Directory.GetParent(dir);
                if (parent != null)
                {
                    var c2 = Path.Combine(parent.FullName, "onnxruntime.dll");
                    if (File.Exists(c2)) return c2;
                }
                return c1;
            }
            catch { return null; }
        }

        private void RefreshStats()
        {
            var s = AppState.Instance;
            TxtStatAudioCount.Text = s.AudioFileCount.ToString();
            TxtStatDuration.Text = s.TotalDurationDisplay ?? "00:00:00";
            TxtStatSliceCount.Text = s.SliceCount.ToString();
            TxtStatProgress.Text = s.ProgressPercent + "%";
            ProgressFlow.Value = s.ProgressPercent;
            TxtProgressDesc.Text = s.ProgressText ?? "";
            TxtStatusBar.Text = string.IsNullOrEmpty(s.ProjectDir)
                ? "就绪 - 请选择文件夹导入音频或直接填写 ASR/导出路径"
                : "工作目录: " + s.ProjectDir;
        }

        private void BtnGoImport_Click(object sender, RoutedEventArgs e)
        {
            TabMain.SelectedIndex = 1;
        }

        private void BtnSelectFiles_Click(object sender, RoutedEventArgs e)
        {
            var dlg = new Microsoft.Win32.OpenFileDialog
            {
                Filter = "音频文件|*.wav;*.mp3;*.flac;*.m4a;*.ogg|全部|*.*",
                Multiselect = true
            };
            if (dlg.ShowDialog() != true) return;
            AddAudioPaths(dlg.FileNames);
        }

        private void BtnSelectFolder_Click(object sender, RoutedEventArgs e)
        {
            var dialog = new System.Windows.Forms.FolderBrowserDialog
            {
                Description = "选择导入音频所在的文件夹（将在此目录下创建 preprocessed、slices、asr_output 子目录）",
                ShowNewFolderButton = true
            };
            if (dialog.ShowDialog() != System.Windows.Forms.DialogResult.OK) return;
            var dir = dialog.SelectedPath;
            var s = AppState.Instance;
            s.ProjectDir = dir;
            s.EnsureProjectDirs();
            s.AddLog("工作目录: " + dir);
            TxtInputDir.Text = Path.Combine(dir, "slices");
            TxtOutputDir.Text = Path.Combine(dir, "asr_output");
            // FA 对齐默认：输入=fa_input（由“导出到 FA 目录”生成），输出=textgrid
            var asrDir = Path.Combine(dir, "asr_output");
            var faInputDir = Path.Combine(dir, "fa_input");
            var textgridDir = Path.Combine(dir, "textgrid");
            TxtFAInputWavs.Text = faInputDir;
            TxtFAOutputTg.Text = textgridDir;
            var exts = new[] { ".wav", ".mp3", ".flac", ".m4a", ".ogg" };
            var files = Directory.GetFiles(dir, "*.*", SearchOption.TopDirectoryOnly)
                .Where(f => exts.Contains(Path.GetExtension(f).ToLowerInvariant())).ToArray();
            AddAudioPaths(files);
            // 如果已有 ASR 输出结果，自动加载到「人工校正」列表
            try
            {
                if (Directory.Exists(asrDir) && Directory.GetFiles(asrDir, "*.txt").Length > 0)
                {
                    RefreshASRResultsFromOutput(asrDir);
                    s.AddLog("检测到已有 ASR 输出，已自动加载到校正列表。");
                }
            }
            catch { }
            RefreshStats();
        }

        private void AddAudioPaths(string[] paths)
        {
            if (paths == null || paths.Length == 0) return;
            var s = AppState.Instance;
            var destDir = s.RawDir;
            foreach (var p in paths)
            {
                if (!File.Exists(p)) continue;
                var name = Path.GetFileName(p);
                var ext = Path.GetExtension(p).ToUpperInvariant().TrimStart('.');
                if (ext == "M4A") ext = "M4A"; else if (ext == "OGG") ext = "OGG"; else if (ext == "FLAC") ext = "FLAC";
                var item = new AudioFileItem
                {
                    FilePath = p,
                    FileName = name,
                    Format = ext,
                    Duration = "—",
                    SampleRate = "—",
                    Status = "待处理",
                    Note = ""
                };
                if (!string.IsNullOrEmpty(destDir))
                {
                    try
                    {
                        var destPath = Path.Combine(destDir, name);
                        if (!File.Exists(destPath) || new FileInfo(p).Length != new FileInfo(destPath).Length)
                            File.Copy(p, destPath, true);
                        item.FilePath = destPath;
                    }
                    catch (Exception ex) { item.Note = ex.Message; }
                }
                s.AudioFiles.Add(item);
            }
            s.AddLog("已添加 " + paths.Length + " 个音频文件");
            RefreshStats();
        }

        private void BtnClearAudioList_Click(object sender, RoutedEventArgs e)
        {
            AppState.Instance.AudioFiles.Clear();
            AppState.Instance.AddLog("已清空音频列表");
            RefreshStats();
        }

        private static readonly string[] AudioExtensions = { ".wav", ".mp3", ".flac", ".m4a", ".ogg" };

        private void DropZoneAudio_DragOver(object sender, DragEventArgs e)
        {
            if (e.Data.GetDataPresent(DataFormats.FileDrop))
            {
                e.Effects = DragDropEffects.Copy;
                e.Handled = true;
            }
        }

        private void DropZoneAudio_Drop(object sender, DragEventArgs e)
        {
            if (!e.Data.GetDataPresent(DataFormats.FileDrop)) return;
            var data = e.Data.GetData(DataFormats.FileDrop);
            var paths = data as string[];
            if (paths == null || paths.Length == 0) return;
            var files = new System.Collections.Generic.List<string>();
            string singleDroppedFolder = null;
            foreach (var p in paths)
            {
                if (string.IsNullOrWhiteSpace(p)) continue;
                try
                {
                    if (File.Exists(p))
                    {
                        var ext = Path.GetExtension(p).ToLowerInvariant();
                        if (AudioExtensions.Contains(ext)) files.Add(p);
                    }
                    else if (Directory.Exists(p))
                    {
                        if (paths.Length == 1) singleDroppedFolder = p;
                        foreach (var f in Directory.GetFiles(p, "*.*", SearchOption.TopDirectoryOnly))
                        {
                            var ext = Path.GetExtension(f).ToLowerInvariant();
                            if (AudioExtensions.Contains(ext)) files.Add(f);
                        }
                    }
                }
                catch { /* 忽略单条错误 */ }
            }
            if (singleDroppedFolder != null)
            {
                AppState.Instance.ProjectDir = singleDroppedFolder;
                AppState.Instance.EnsureProjectDirs();
                AppState.Instance.AddLog("工作目录: " + singleDroppedFolder);
                var slicesDir = Path.Combine(singleDroppedFolder, "slices");
                var asrDir = Path.Combine(singleDroppedFolder, "asr_output");
                var faInputDir = Path.Combine(singleDroppedFolder, "fa_input");
                var textgridDir = Path.Combine(singleDroppedFolder, "textgrid");
                TxtInputDir.Text = slicesDir;
                TxtOutputDir.Text = asrDir;
                // FA 输入 / 输出目录也一并填好（输入默认 fa_input）
                TxtFAInputWavs.Text = faInputDir;
                TxtFAOutputTg.Text = textgridDir;
                // 如果已有 ASR 输出结果，则尝试自动加载
                try
                {
                    if (Directory.Exists(asrDir) && Directory.GetFiles(asrDir, "*.txt").Length > 0)
                    {
                        RefreshASRResultsFromOutput(asrDir);
                        AppState.Instance.AddLog("检测到已有 ASR 输出，已自动加载到校正列表。");
                    }
                }
                catch { }
            }
            if (files.Count > 0)
                AddAudioPaths(files.ToArray());
            e.Handled = true;
        }

        private void TxtSearchAudio_TextChanged(object sender, TextChangedEventArgs e)
        {
            var filter = (TxtSearchAudio?.Text ?? "").Trim().ToLowerInvariant();
            var view = CollectionViewSource.GetDefaultView(AppState.Instance.AudioFiles);
            if (string.IsNullOrEmpty(filter))
                view.Filter = null;
            else
                view.Filter = obj => (obj is AudioFileItem a) && (a.FileName ?? "").ToLowerInvariant().Contains(filter);
        }

        private async void BtnPreprocess_Click(object sender, RoutedEventArgs e)
        {
            var s = AppState.Instance;
            if (string.IsNullOrEmpty(s.RawDir) || !Directory.Exists(s.RawDir))
            {
                MessageBox.Show("请先在「音频导入」页选择文件夹导入音频。", "提示", MessageBoxButton.OK, MessageBoxImage.Information);
                return;
            }
            if (!File.Exists(_goExePath))
            {
                TxtPreprocessStatus.Text = "未找到 DataForgeLite.exe，请先编译 Go 项目。";
                return;
            }
            double targetLufs = -18;
            double truePeakLimit = -1;
            double.TryParse(TxtTargetLufs?.Text?.Trim(), System.Globalization.NumberStyles.Any, System.Globalization.CultureInfo.InvariantCulture, out targetLufs);
            double.TryParse(TxtTruePeakLimit?.Text?.Trim(), System.Globalization.NumberStyles.Any, System.Globalization.CultureInfo.InvariantCulture, out truePeakLimit);
            if (truePeakLimit > 0) truePeakLimit = -1;
            BtnPreprocess.IsEnabled = false;
            TxtPreprocessStatus.Text = "正在调用 Go 后端预处理...";
            try
            {
                var runResult = await Task.Run(() => DataForgeLiteBackend.RunPreprocess(s.RawDir, s.PreprocessedDir, targetLufs, truePeakLimit, _goExePath));
                var outObj = runResult.Output;
                if (outObj != null)
                {
                    if (outObj.Success)
                    {
                        TxtPreprocessStatus.Text = string.Format("预处理完成！成功: {0}, 失败: {1}", outObj.SuccessCount, outObj.ErrorCount);
                        s.AddLog("预处理完成: 成功 " + outObj.SuccessCount + ", 失败 " + outObj.ErrorCount);
                    }
                    else
                        TxtPreprocessStatus.Text = "预处理失败：" + (outObj.Error ?? runResult.Stderr ?? "");
                }
                else
                    TxtPreprocessStatus.Text = "调用失败：" + (runResult.Stderr ?? "");
            }
            catch (Exception ex) { TxtPreprocessStatus.Text = "错误：" + ex.Message; }
            finally { BtnPreprocess.IsEnabled = true; }
        }

        private async void BtnSplit_Click(object sender, RoutedEventArgs e)
        {
            var s = AppState.Instance;
            var inputDir = s.PreprocessedDir ?? s.RawDir;
            var outputDir = s.SlicesDir;
            if (string.IsNullOrEmpty(inputDir) || !Directory.Exists(inputDir))
            {
                MessageBox.Show("请先选择文件夹导入音频（或先执行预处理）。", "提示", MessageBoxButton.OK, MessageBoxImage.Information);
                return;
            }
            if (string.IsNullOrEmpty(outputDir)) outputDir = Path.Combine(Path.GetDirectoryName(inputDir), "slices");
            if (!File.Exists(_goExePath))
            {
                TxtPreprocessStatus.Text = "未找到 DataForgeLite.exe，请先编译 Go 项目。";
                return;
            }
            BtnSplit.IsEnabled = false;
            TxtPreprocessStatus.Text = "正在调用 Go 后端切分...";
            try
            {
                var runResult = await Task.Run(() => DataForgeLiteBackend.RunSplit(inputDir, outputDir, _goExePath));
                var outObj = runResult.Output;
                if (outObj != null)
                {
                    if (outObj.Success)
                    {
                        TxtPreprocessStatus.Text = string.Format("切分完成！成功: {0}, 失败: {1}", outObj.SuccessCount, outObj.ErrorCount);
                        s.AddLog("切分完成: 成功 " + outObj.SuccessCount + ", 失败 " + outObj.ErrorCount);
                        TxtInputDir.Text = outputDir;
                    }
                    else
                        TxtPreprocessStatus.Text = "切分失败：" + (outObj.Error ?? runResult.Stderr ?? "");
                }
                else
                    TxtPreprocessStatus.Text = "调用失败：" + (runResult.Stderr ?? "");
            }
            catch (Exception ex) { TxtPreprocessStatus.Text = "错误：" + ex.Message; }
            finally { BtnSplit.IsEnabled = true; }
        }

        private async void BtnStartASR_Click(object sender, RoutedEventArgs e)
        {
            string inputDir = (TxtInputDir?.Text ?? "").Trim();
            string outputDir = (TxtOutputDir?.Text ?? "").Trim();
            if (string.IsNullOrWhiteSpace(inputDir)) { TxtASRStatus.Text = "错误：请选择输入文件夹"; return; }
            if (string.IsNullOrWhiteSpace(outputDir)) { TxtASRStatus.Text = "错误：请选择输出文件夹"; return; }
            if (!Directory.Exists(inputDir)) { TxtASRStatus.Text = "错误：输入文件夹不存在"; return; }
            if (!File.Exists(_goExePath))
            {
                TxtASRStatus.Text = "错误：未找到 DataForgeLite.exe，请先编译 Go 项目并将 exe 放在本程序同目录或上级目录。";
                return;
            }
            // 统一为绝对路径，确保后端写入与前端刷新读的是同一目录
            try
            {
                outputDir = Path.GetFullPath(outputDir);
                inputDir = Path.GetFullPath(inputDir);
            }
            catch { }
            BtnStartASR.IsEnabled = false;
            ProgressASR.Visibility = System.Windows.Visibility.Visible;
            TxtASRStatus.Text = "正在调用 Go 后端识别中，请稍候...";
            try
            {
                var language = GetAsrLanguageOption();
                var backend = GetAsrBackendOption();
                ApplyOrtDevice();
                var runResult = await Task.Run(() => DataForgeLiteBackend.RunAsr(inputDir, outputDir, language, _goExePath, backend));
                var outObj = runResult.Output;
                var stderr = runResult.Stderr ?? "";
                var outPath = outputDir;
                // 全部在 UI 线程更新状态并刷新表格，确保推理结果能显示
                Dispatcher.Invoke(() =>
                {
                    if (outObj != null)
                    {
                        if (outObj.Success)
                        {
                            TxtASRStatus.Text = string.Format("识别完成！成功: {0}, 失败: {1}", outObj.SuccessCount, outObj.ErrorCount);
                            AppState.Instance.AddLog("ASR 完成: 成功 " + outObj.SuccessCount + ", 失败 " + outObj.ErrorCount);
                            RefreshASRResultsFromOutput(outPath);
                            // ASR 成功后，自动为 FA 对齐填好输入/输出目录：
                            // 输入 = fa_input（若有 ProjectDir 则在项目目录下）；输出 = 项目目录下或同级目录下的 textgrid
                            var sLocal = AppState.Instance;
                            var faParentDir = !string.IsNullOrWhiteSpace(sLocal.ProjectDir) && Directory.Exists(sLocal.ProjectDir)
                                ? sLocal.ProjectDir
                                : Path.GetDirectoryName(outPath);
                            if (!string.IsNullOrWhiteSpace(faParentDir))
                            {
                                var faInputDir = Path.Combine(faParentDir, "fa_input");
                                var textgridDir = Path.Combine(faParentDir, "textgrid");
                                TxtFAInputWavs.Text = faInputDir;
                                TxtFAOutputTg.Text = textgridDir;
                            }
                        }
                        else
                            TxtASRStatus.Text = "识别失败：" + (outObj.Error ?? stderr ?? "未知错误");
                    }
                    else
                        TxtASRStatus.Text = "调用失败：" + (stderr ?? "无法解析后端输出");
                });
            }
            catch (Exception ex)
            {
                Dispatcher.Invoke(() => TxtASRStatus.Text = "错误：" + ex.Message);
            }
            finally
            {
                Dispatcher.Invoke(() => {
                    BtnStartASR.IsEnabled = true;
                    ProgressASR.Visibility = System.Windows.Visibility.Collapsed;
                });
            }
        }

        private void RefreshASRResultsFromOutput(string outputDir)
        {
            if (string.IsNullOrWhiteSpace(outputDir) || !Directory.Exists(outputDir)) return;
            try { outputDir = Path.GetFullPath(outputDir); } catch { }
            var s = AppState.Instance;
            s.ASRResults.Clear();
            s.CorrectionList.Clear();
            // 只加载主文本文件（排除 *_pinyin.txt），与 ASR 输出命名一致：output_xxx.txt
            var allTxt = Directory.GetFiles(outputDir, "*.txt").OrderBy(Path.GetFileName).ToArray();
            var txtFiles = allTxt.Where(f => !Path.GetFileNameWithoutExtension(f).EndsWith("_pinyin", StringComparison.OrdinalIgnoreCase)).ToArray();
            foreach (var txtPath in txtFiles)
            {
                var baseName = Path.GetFileNameWithoutExtension(txtPath);
                if (baseName.IndexOf("summary", StringComparison.OrdinalIgnoreCase) >= 0) continue;
                var wavPath = Path.Combine(outputDir, baseName + ".wav");
                if (!File.Exists(wavPath)) wavPath = Path.Combine(Path.GetDirectoryName(txtPath) ?? outputDir, baseName + ".wav");
                // 不要求 wav 一定存在才显示：ASR 输出目录通常只有 txt/pinyin/lab，wav 多在 slices，导出 FA 时会按需用 slices 补全
                var text = File.Exists(txtPath) ? File.ReadAllText(txtPath).Trim() : "";
                var dir = Path.GetDirectoryName(txtPath);
                var pinyinPath = Path.Combine(dir ?? outputDir, baseName + "_pinyin.txt");
                var pinyinText = File.Exists(pinyinPath) ? File.ReadAllText(pinyinPath).Trim() : "";
                s.ASRResults.Add(new ASRResultItem
                {
                    SliceId = baseName,
                    RawText = text,
                    ProcessedText = text,
                    Pinyin = pinyinText,
                    Language = "",
                    Confidence = "",
                    Status = "已完成",
                    FilePath = wavPath
                });
                var display = baseName + "  " + (text.Length > 20 ? text.Substring(0, 20) + "…" : text) + "  已完成";
                s.CorrectionList.Add(new CorrectionItem
                {
                    DisplayName = display,
                    TimeRange = "",
                    Status = "已完成",
                    Text = text,
                    Pinyin = pinyinText,
                    WavPath = wavPath,
                    TxtPath = txtPath
                });
            }
            AppState.Instance.AddLog("已加载 " + s.ASRResults.Count + " 条识别结果到校正列表");
            RefreshStats();
        }

        /// <summary>获取当前选择的 ASR 语言选项：auto / Chinese / English，未选时默认 Chinese</summary>
        private string GetAsrLanguageOption()
        {
            var item = CmbAsrLanguage?.SelectedItem as System.Windows.Controls.ComboBoxItem;
            var tag = item?.Tag as string;
            return string.IsNullOrWhiteSpace(tag) ? "Chinese" : tag;
        }

        /// <summary>获取当前选择的 ASR 推理后端：onnx / python，未选时默认 onnx</summary>
        private string GetAsrBackendOption()
        {
            var item = CmbAsrBackend?.SelectedItem as System.Windows.Controls.ComboBoxItem;
            var tag = item?.Tag as string;
            return string.IsNullOrWhiteSpace(tag) ? "onnx" : tag;
        }

        private string GetOrtDevice()
        {
            var item = CmbOrtDevice?.SelectedItem as System.Windows.Controls.ComboBoxItem;
            var tag = item?.Tag as string;
            return string.IsNullOrWhiteSpace(tag) ? "cpu" : tag;
        }

        private void ApplyOrtDevice()
        {
            var device = GetOrtDevice();
            System.Environment.SetEnvironmentVariable("ORT_DEVICE", device == "cpu" ? null : device);
            if (device == "dml")
            {
                var idItem = CmbDmlDeviceId?.SelectedItem as System.Windows.Controls.ComboBoxItem;
                var idTag = idItem?.Tag as string ?? "0";
                System.Environment.SetEnvironmentVariable("ORT_DML_DEVICE_ID", idTag);
            }
            else
            {
                System.Environment.SetEnvironmentVariable("ORT_DML_DEVICE_ID", null);
            }
        }

        private void CmbOrtDevice_SelectionChanged(object sender, System.Windows.Controls.SelectionChangedEventArgs e)
        {
            if (CmbDmlDeviceId == null) return;
            var item = CmbOrtDevice?.SelectedItem as System.Windows.Controls.ComboBoxItem;
            var tag = item?.Tag as string;
            if (tag == "dml")
            {
                PopulateDmlDeviceList();
                CmbDmlDeviceId.Visibility = System.Windows.Visibility.Visible;
            }
            else
            {
                CmbDmlDeviceId.Visibility = System.Windows.Visibility.Collapsed;
            }
        }

        private void PopulateDmlDeviceList()
        {
            if (CmbDmlDeviceId.Items.Count > 0) return; // 已填充
            try
            {
                var searcher = new System.Management.ManagementObjectSearcher(
                    "SELECT DeviceID, Name FROM Win32_VideoController");
                int idx = 0;
                foreach (System.Management.ManagementObject obj in searcher.Get())
                {
                    var name = obj["Name"]?.ToString() ?? ("GPU " + idx);
                    var cbi = new System.Windows.Controls.ComboBoxItem
                    {
                        Content = string.Format("[{0}] {1}", idx, name),
                        Tag = idx.ToString()
                    };
                    CmbDmlDeviceId.Items.Add(cbi);
                    idx++;
                }
            }
            catch { }
            if (CmbDmlDeviceId.Items.Count == 0)
            {
                CmbDmlDeviceId.Items.Add(new System.Windows.Controls.ComboBoxItem { Content = "[0] 默认 GPU", Tag = "0" });
            }
            CmbDmlDeviceId.SelectedIndex = 0;
        }

        private void BtnRefreshASRResults_Click(object sender, RoutedEventArgs e)
        {
            var outputDir = (TxtOutputDir?.Text ?? "").Trim();
            if (string.IsNullOrEmpty(outputDir)) { MessageBox.Show("请先填写输出文件夹路径。", "提示", MessageBoxButton.OK, MessageBoxImage.Information); return; }
            if (!Directory.Exists(outputDir)) { MessageBox.Show("输出文件夹不存在。", "提示", MessageBoxButton.OK, MessageBoxImage.Warning); return; }
            // 刷新 ASR 结果时，顺带为 FA 对齐填好默认输入/输出目录
            try
            {
                outputDir = Path.GetFullPath(outputDir);
            }
            catch { }
            var s = AppState.Instance;
            var faParentDir = !string.IsNullOrWhiteSpace(s.ProjectDir) && Directory.Exists(s.ProjectDir)
                ? s.ProjectDir
                : Path.GetDirectoryName(outputDir);
            if (!string.IsNullOrWhiteSpace(faParentDir))
            {
                var faInputDir = Path.Combine(faParentDir, "fa_input");
                var textgridDir = Path.Combine(faParentDir, "textgrid");
                TxtFAInputWavs.Text = faInputDir;
                TxtFAOutputTg.Text = textgridDir;
            }
            RefreshASRResultsFromOutput(outputDir);
        }

        /// <summary>打开当前选择的 ASR 输出目录</summary>
        private void BtnOpenASRDebug_Click(object sender, RoutedEventArgs e)
        {
            var outputDir = (TxtOutputDir?.Text ?? "").Trim();
            if (string.IsNullOrWhiteSpace(outputDir) || !Directory.Exists(outputDir))
            {
                MessageBox.Show("请先选择 ASR 输出文件夹，或确认该目录存在。", "提示", MessageBoxButton.OK, MessageBoxImage.Information);
                return;
            }
            try { Process.Start("explorer.exe", outputDir); }
            catch (Exception ex) { MessageBox.Show("无法打开资源管理器：" + ex.Message, "错误", MessageBoxButton.OK, MessageBoxImage.Warning); }
        }

        /// <summary>在命令行窗口运行 ASR，便于查看实时输出（调试用）</summary>
        private void BtnRunASRInConsole_Click(object sender, RoutedEventArgs e)
        {
            string inputDir = (TxtInputDir?.Text ?? "").Trim();
            string outputDir = (TxtOutputDir?.Text ?? "").Trim();
            if (string.IsNullOrWhiteSpace(inputDir)) { MessageBox.Show("请先选择输入文件夹。", "提示", MessageBoxButton.OK, MessageBoxImage.Information); return; }
            if (string.IsNullOrWhiteSpace(outputDir)) { MessageBox.Show("请先选择输出文件夹。", "提示", MessageBoxButton.OK, MessageBoxImage.Information); return; }
            if (!File.Exists(_goExePath)) { MessageBox.Show("未找到 DataForgeLite.exe。", "提示", MessageBoxButton.OK, MessageBoxImage.Warning); return; }
            var lang = GetAsrLanguageOption();
            var backend = GetAsrBackendOption();
            var args = string.Format("--asr --input \"{0}\" --output \"{1}\"", inputDir, outputDir);
            if (!string.IsNullOrWhiteSpace(lang)) args += string.Format(" --language \"{0}\"", lang);
            args += string.Format(" --backend {0}", backend);
            try
            {
                Process.Start(new ProcessStartInfo
                {
                    FileName = "cmd.exe",
                    Arguments = "/k \"\"" + _goExePath + "\" " + args + " & echo. & pause\"",
                    WorkingDirectory = Path.GetDirectoryName(_goExePath),
                    CreateNoWindow = false,
                    UseShellExecute = true
                });
            }
            catch (Exception ex) { MessageBox.Show("无法启动命令行窗口：" + ex.Message, "错误", MessageBoxButton.OK, MessageBoxImage.Error); }
        }

        private void BtnBrowseInput_Click(object sender, RoutedEventArgs e)
        {
            var dialog = new System.Windows.Forms.FolderBrowserDialog
            {
                Description = "选择待识别音频文件夹（支持 .wav, .mp3, .flac 等）",
                ShowNewFolderButton = false
            };
            if (dialog.ShowDialog() == System.Windows.Forms.DialogResult.OK)
                TxtInputDir.Text = dialog.SelectedPath;
        }

        private void BtnBrowseOutput_Click(object sender, RoutedEventArgs e)
        {
            var dialog = new System.Windows.Forms.FolderBrowserDialog
            {
                Description = "选择识别结果输出文件夹",
                ShowNewFolderButton = true
            };
            if (dialog.ShowDialog() == System.Windows.Forms.DialogResult.OK)
                TxtOutputDir.Text = dialog.SelectedPath;
        }

        private void BtnExportToFADir_Click(object sender, RoutedEventArgs e)
        {
            var s = AppState.Instance;
            var asrOutputDir = (TxtOutputDir?.Text ?? "").Trim();
            var slicesDir = s.SlicesDir;
            if (string.IsNullOrEmpty(asrOutputDir) || !Directory.Exists(asrOutputDir))
            {
                MessageBox.Show("请先填写 ASR 输出目录。", "提示", MessageBoxButton.OK, MessageBoxImage.Information);
                return;
            }
            string parentDir = !string.IsNullOrWhiteSpace(s.ProjectDir) && Directory.Exists(s.ProjectDir)
                ? s.ProjectDir
                : Path.GetDirectoryName(asrOutputDir);
            var faInputDir = Path.Combine(parentDir, "fa_input");
            try
            {
                Directory.CreateDirectory(faInputDir);
                var sliceWavMap = new System.Collections.Generic.Dictionary<string, string>(StringComparer.OrdinalIgnoreCase);
                if (!string.IsNullOrEmpty(slicesDir) && Directory.Exists(slicesDir))
                    foreach (var f in Directory.GetFiles(slicesDir, "*.wav"))
                        sliceWavMap[Path.GetFileNameWithoutExtension(f)] = f;
                var copied = 0; var missed = 0;
                foreach (var labFile in Directory.GetFiles(asrOutputDir, "*.lab"))
                {
                    var baseName = Path.GetFileNameWithoutExtension(labFile);
                    File.Copy(labFile, Path.Combine(faInputDir, baseName + ".lab"), true);
                    var wavDst = Path.Combine(faInputDir, baseName + ".wav");
                    // 先按同名找，再去掉 output_ 前缀找
                    string wavSrc = null;
                    if (sliceWavMap.TryGetValue(baseName, out wavSrc)) { }
                    else
                    {
                        var stripped = baseName.StartsWith("output_") ? baseName.Substring(7) : baseName;
                        sliceWavMap.TryGetValue(stripped, out wavSrc);
                    }
                    if (wavSrc != null) { File.Copy(wavSrc, wavDst, true); copied++; }
                    else missed++;
                }
                TxtFAInputWavs.Text = faInputDir;
                TxtFAOutputTg.Text = Path.Combine(parentDir, "textgrid");
                Directory.CreateDirectory(TxtFAOutputTg.Text);
                MessageBox.Show(string.Format("已复制 {0} 条到 {1}，缺 WAV: {2}", copied, faInputDir, missed), "导出完成", MessageBoxButton.OK, MessageBoxImage.Information);
            }
            catch (Exception ex) { MessageBox.Show("导出失败: " + ex.Message, "错误", MessageBoxButton.OK, MessageBoxImage.Error); }
        }

        private void BtnBrowseExportInDir_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择输入目录（含 wavs/ 和 TextGrid/ 子目录）" };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK) TxtExportInDir.Text = d.SelectedPath;
        }

        private void BtnBrowseExportWavs_Click(object sender, RoutedEventArgs e) { }
        private void BtnBrowseExportTg_Click(object sender, RoutedEventArgs e) { }

        private void BtnBrowseExportOut_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择导出输出目录", ShowNewFolderButton = true };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK) TxtExportOut.Text = d.SelectedPath;
        }

        private void BtnBrowseFAModelDir_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择 HubertFA 模型目录（含 config 与 ONNX）", ShowNewFolderButton = false };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK)
            {
                TxtFAModelDir.Text = d.SelectedPath;
                LoadHFAModelCapabilities(d.SelectedPath);
            }
        }

        private void BtnBrowseFAInputWavs_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择输入目录（含切分后的音频与同名 .lab 文件）", ShowNewFolderButton = false };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK) TxtFAInputWavs.Text = d.SelectedPath;
        }

        private void BtnBrowseFAOutputTg_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择 TextGrid 输出目录", ShowNewFolderButton = true };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK) TxtFAOutputTg.Text = d.SelectedPath;
        }

        private string GetFaLanguage()
        {
            if (PanelFaLanguages == null) return "zh";
            foreach (var child in PanelFaLanguages.Children)
            {
                var rb = child as RadioButton;
                if (rb != null && rb.IsChecked == true)
                    return rb.Tag as string ?? "zh";
            }
            return "zh";
        }

        private string GetFaNonLexical()
        {
            if (PanelFaNonLexical == null) return "";
            var list = new System.Collections.Generic.List<string>();
            foreach (var child in PanelFaNonLexical.Children)
            {
                var cb = child as CheckBox;
                if (cb != null && cb.IsChecked == true)
                    list.Add(cb.Tag as string ?? cb.Content as string ?? "");
            }
            return string.Join(",", list);
        }

        /// <summary>
        /// 从 hfa_model 的 vocab.json 读取模型能力，动态填充语言和非词汇音素选项。
        /// </summary>
        private void LoadHFAModelCapabilities(string modelDir)
        {
            PanelFaLanguages?.Children.Clear();
            PanelFaNonLexical?.Children.Clear();
            if (TxtFAModelInfo != null) TxtFAModelInfo.Text = "";

            if (string.IsNullOrWhiteSpace(modelDir) || !Directory.Exists(modelDir))
            {
                // 模型目录无效时显示默认选项
                AddDefaultFaOptions();
                return;
            }

            var vocabPath = Path.Combine(modelDir, "vocab.json");
            if (!File.Exists(vocabPath))
            {
                AddDefaultFaOptions();
                if (TxtFAModelInfo != null) TxtFAModelInfo.Text = "未找到 vocab.json，使用默认选项";
                return;
            }

            try
            {
                var json = File.ReadAllText(vocabPath);
                // 简易 JSON 解析（.NET 4.6.2 无 System.Text.Json，用 DataContractJsonSerializer）
                var vocabData = ParseVocabJson(json);

                // 填充语言列表
                if (vocabData.Dictionaries != null && vocabData.Dictionaries.Count > 0)
                {
                    bool first = true;
                    // 预定义排序：zh 优先
                    var langOrder = new[] { "zh", "yue", "ja", "ko", "en" };
                    var sorted = new System.Collections.Generic.List<string>();
                    foreach (var l in langOrder)
                        if (vocabData.Dictionaries.ContainsKey(l)) sorted.Add(l);
                    foreach (var l in vocabData.Dictionaries.Keys)
                        if (!sorted.Contains(l)) sorted.Add(l);

                    foreach (var lang in sorted)
                    {
                        var rb = new RadioButton
                        {
                            Content = lang,
                            Tag = lang,
                            GroupName = "FaLang",
                            IsChecked = first,
                            Margin = new Thickness(0, 0, 12, 0)
                        };
                        PanelFaLanguages?.Children.Add(rb);
                        first = false;
                    }
                }
                else
                {
                    AddDefaultLanguageOption();
                }

                // 填充非词汇音素列表
                if (vocabData.NonLexicalPhonemes != null && vocabData.NonLexicalPhonemes.Length > 0)
                {
                    foreach (var ph in vocabData.NonLexicalPhonemes)
                    {
                        var cb = new CheckBox
                        {
                            Content = ph,
                            Tag = ph,
                            IsChecked = true,
                            Margin = new Thickness(0, 0, 16, 0)
                        };
                        PanelFaNonLexical?.Children.Add(cb);
                    }
                }

                if (TxtFAModelInfo != null)
                {
                    var langCount = vocabData.Dictionaries?.Count ?? 0;
                    var nlCount = vocabData.NonLexicalPhonemes?.Length ?? 0;
                    TxtFAModelInfo.Text = string.Format("模型已加载: {0} 种语言, {1} 种非词汇音素, vocab_size={2}",
                        langCount, nlCount, vocabData.VocabSize);
                }
            }
            catch (Exception ex)
            {
                AddDefaultFaOptions();
                if (TxtFAModelInfo != null) TxtFAModelInfo.Text = "读取 vocab.json 失败: " + ex.Message;
            }
        }

        private void AddDefaultFaOptions()
        {
            AddDefaultLanguageOption();
            // 默认非词汇音素
            foreach (var ph in new[] { "AP", "EP" })
            {
                var cb = new CheckBox { Content = ph, Tag = ph, IsChecked = true, Margin = new Thickness(0, 0, 16, 0) };
                PanelFaNonLexical?.Children.Add(cb);
            }
        }

        private void AddDefaultLanguageOption()
        {
            var rb = new RadioButton { Content = "zh", Tag = "zh", GroupName = "FaLang", IsChecked = true, Margin = new Thickness(0, 0, 12, 0) };
            PanelFaLanguages?.Children.Add(rb);
        }

        /// <summary>vocab.json 解析结果</summary>
        private class VocabData
        {
            public System.Collections.Generic.Dictionary<string, string> Dictionaries;
            public string[] NonLexicalPhonemes;
            public int VocabSize;
        }

        /// <summary>
        /// 简易解析 vocab.json（兼容 .NET 4.6.2，不依赖 Newtonsoft）。
        /// DataContractJsonSerializer 需要 UseSimpleDictionaryFormat 才能解析 {"key":"value"} 格式的字典。
        /// </summary>
        private static VocabData ParseVocabJson(string json)
        {
            var result = new VocabData
            {
                Dictionaries = new System.Collections.Generic.Dictionary<string, string>(),
                NonLexicalPhonemes = new string[0],
                VocabSize = 0
            };

            var settings = new System.Runtime.Serialization.Json.DataContractJsonSerializerSettings
            {
                UseSimpleDictionaryFormat = true
            };
            var ser = new System.Runtime.Serialization.Json.DataContractJsonSerializer(typeof(VocabJsonRaw), settings);
            using (var ms = new System.IO.MemoryStream(System.Text.Encoding.UTF8.GetBytes(json)))
            {
                var raw = (VocabJsonRaw)ser.ReadObject(ms);
                if (raw != null)
                {
                    if (raw.dictionaries != null)
                        result.Dictionaries = new System.Collections.Generic.Dictionary<string, string>(raw.dictionaries);
                    if (raw.non_lexical_phonemes != null)
                        result.NonLexicalPhonemes = raw.non_lexical_phonemes;
                    result.VocabSize = raw.vocab_size;
                }
            }

            return result;
        }

        [System.Runtime.Serialization.DataContract]
        private class VocabJsonRaw
        {
            [System.Runtime.Serialization.DataMember] public System.Collections.Generic.Dictionary<string, string> dictionaries;
            [System.Runtime.Serialization.DataMember] public string[] non_lexical_phonemes;
            [System.Runtime.Serialization.DataMember] public int vocab_size;
        }

        private async void BtnStartFA_Click(object sender, RoutedEventArgs e)
        {
            var modelDir = (TxtFAModelDir?.Text ?? "").Trim();
            var inputDir = (TxtFAInputWavs?.Text ?? "").Trim();
            var outputDir = (TxtFAOutputTg?.Text ?? "").Trim();
            if (string.IsNullOrWhiteSpace(inputDir)) { TxtFAStatus.Text = "请填写输入目录（含 .wav 与同名 .lab）"; return; }
            if (string.IsNullOrWhiteSpace(outputDir)) { TxtFAStatus.Text = "请填写 TextGrid 输出目录"; return; }
            if (!Directory.Exists(inputDir)) { TxtFAStatus.Text = "输入目录不存在"; return; }
            if (string.IsNullOrWhiteSpace(modelDir) || !Directory.Exists(modelDir)) { TxtFAStatus.Text = "模型目录不存在"; return; }
            if (!File.Exists(_goExePath))
            {
                TxtFAStatus.Text = "错误：未找到 DataForgeLite.exe，请先编译 Go 项目。";
                return;
            }

            var language = GetFaLanguage();
            var nonLex = GetFaNonLexical();
            ApplyOrtDevice();

            BtnStartFA.IsEnabled = false;
            ProgressFA.Visibility = System.Windows.Visibility.Visible;
            TxtFAStatus.Text = "正在调用 Go 后端进行 FA 对齐，请稍候...";
            try
            {
                var runResult = await Task.Run(() => DataForgeLiteBackend.RunFA(modelDir, inputDir, outputDir, language, nonLex, _goExePath));
                var outObj = runResult.Output;
                var stderr = runResult.Stderr ?? "";

                Dispatcher.Invoke(() =>
                {
                    if (outObj != null)
                    {
                        if (outObj.Success)
                        {
                            TxtFAStatus.Text = string.Format("FA 对齐完成！成功: {0}, 失败: {1}", outObj.SuccessCount, outObj.ErrorCount);
                            AppState.Instance.AddLog("FA 对齐完成: 成功 " + outObj.SuccessCount + ", 失败 " + outObj.ErrorCount);
                            TxtExportInDir.Text = System.IO.Path.GetDirectoryName(inputDir);
                        }
                        else
                            TxtFAStatus.Text = "FA 对齐失败：" + (outObj.Error ?? stderr ?? "未知错误");
                    }
                    else
                        TxtFAStatus.Text = "调用失败：" + (stderr ?? "无法解析后端输出");
                });
            }
            catch (Exception ex)
            {
                Dispatcher.Invoke(() => TxtFAStatus.Text = "FA 对齐失败: " + ex.Message);
            }
            finally
            {
                Dispatcher.Invoke(() => {
                    BtnStartFA.IsEnabled = true;
                    ProgressFA.Visibility = System.Windows.Visibility.Collapsed;
                });
            }
        }

        // RunFaWithPython and FindUvExe have been removed.
        // FA now runs via DataForgeLiteBackend.RunFA() through the Go backend.

        private void BtnBrowseAddPhNumInput_Click(object sender, RoutedEventArgs e) { }
        private void BtnBrowseAddPhNumOutput_Click(object sender, RoutedEventArgs e) { }

        private void BtnBrowseAddPhNumDict_Click(object sender, RoutedEventArgs e)
        {
            var d = new Microsoft.Win32.OpenFileDialog { Filter = "词典|*.txt|全部|*.*", Title = "选择音素词典文件" };
            if (d.ShowDialog() == true) TxtAddPhNumDict.Text = d.FileName;
        }

        private void BtnAddPhNum_Click(object sender, RoutedEventArgs e) { }

        private void BtnBrowseCombineWavs_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择 WAV 目录" };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK) TxtCombineWavs.Text = d.SelectedPath;
        }

        private void BtnBrowseCombineTg_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择 TextGrid 目录（留空则同 WAV 目录）" };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK) TxtCombineTg.Text = d.SelectedPath;
        }

        private void BtnBrowseCombineOut_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择合并输出目录（将自动创建 wavs/ 和 TextGrid/ 子目录）", ShowNewFolderButton = true };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK) TxtCombineOut.Text = d.SelectedPath;
        }

        private async void BtnCombineTg_Click(object sender, RoutedEventArgs e)
        {
            var wavs = (TxtCombineWavs?.Text ?? "").Trim();
            var tg = (TxtCombineTg?.Text ?? "").Trim();
            var outDir = (TxtCombineOut?.Text ?? "").Trim();
            var suffix = (TxtCombineSuffix?.Text ?? @"_\d+").Trim();
            var overwrite = ChkCombineOverwrite?.IsChecked == true;
            if (string.IsNullOrEmpty(wavs) || string.IsNullOrEmpty(outDir))
            {
                TxtCombineStatus.Text = "请填写 WAV 目录和输出目录。";
                return;
            }
            if (!Directory.Exists(wavs)) { TxtCombineStatus.Text = "WAV 目录不存在"; return; }
            if (!File.Exists(_goExePath)) { TxtCombineStatus.Text = "未找到 DataForgeLite.exe"; return; }
            TxtCombineStatus.Text = "正在合并...";
            BtnCombineTg.IsEnabled = false;
            try
            {
                var runResult = await Task.Run(() => DataForgeLiteBackend.RunCombine(wavs, tg, outDir, suffix, overwrite, _goExePath));
                var outObj = runResult.Output;
                if (outObj != null && outObj.Success)
                    TxtCombineStatus.Text = "合并完成！输出: " + outDir;
                else
                    TxtCombineStatus.Text = "合并失败: " + (outObj?.Error ?? runResult.Stderr ?? "未知错误");
            }
            catch (Exception ex) { TxtCombineStatus.Text = "错误: " + ex.Message; }
            finally { BtnCombineTg.IsEnabled = true; }
        }

        private void BtnBrowseSliceIn_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择输入父目录（含 wavs/ 和 TextGrid/ 子目录）" };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK) TxtSliceIn.Text = d.SelectedPath;
        }

        private void BtnBrowseSliceOut_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择拆分输出目录", ShowNewFolderButton = true };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK) TxtSliceOut.Text = d.SelectedPath;
        }

        private async void BtnSliceTg_Click(object sender, RoutedEventArgs e)
        {
            var inDir = (TxtSliceIn?.Text ?? "").Trim();
            var outDir = (TxtSliceOut?.Text ?? "").Trim();
            var preserveName = ChkSlicePreserveName?.IsChecked == true;
            var overwrite = ChkSliceOverwrite?.IsChecked == true;
            if (!int.TryParse(TxtSliceDigits?.Text ?? "3", out int digits)) digits = 3;
            if (string.IsNullOrEmpty(inDir) || string.IsNullOrEmpty(outDir))
            {
                TxtSliceStatus.Text = "请填写输入目录和输出目录。";
                return;
            }
            if (!Directory.Exists(inDir)) { TxtSliceStatus.Text = "输入目录不存在"; return; }
            if (!File.Exists(_goExePath)) { TxtSliceStatus.Text = "未找到 DataForgeLite.exe"; return; }
            TxtSliceStatus.Text = "正在拆分...";
            BtnSliceTg.IsEnabled = false;
            try
            {
                var runResult = await Task.Run(() => DataForgeLiteBackend.RunSliceTg(inDir, outDir, digits, preserveName, overwrite, _goExePath));
                var outObj = runResult.Output;
                if (outObj != null && outObj.Success)
                    TxtSliceStatus.Text = "拆分完成！输出: " + outDir;
                else
                    TxtSliceStatus.Text = "拆分失败: " + (outObj?.Error ?? runResult.Stderr ?? "未知错误");
            }
            catch (Exception ex) { TxtSliceStatus.Text = "错误: " + ex.Message; }
            finally { BtnSliceTg.IsEnabled = true; }
        }

        private async void BtnExportDataset_Click(object sender, RoutedEventArgs e)
        {
            var inDir = (TxtExportInDir?.Text ?? "").Trim();
            var outDir = (TxtExportOut?.Text ?? "").Trim();
            var dictPath = (TxtAddPhNumDict?.Text ?? "").Trim();
            if (string.IsNullOrEmpty(inDir) || string.IsNullOrEmpty(outDir))
            {
                TxtExportStatus.Text = "请填写输入目录和输出目录。";
                return;
            }
            var wavsDir = Path.Combine(inDir, "wavs");
            var tgDir = Path.Combine(inDir, "TextGrid");
            if (!Directory.Exists(wavsDir)) { TxtExportStatus.Text = "输入目录下未找到 wavs/ 子目录"; return; }
            if (!Directory.Exists(tgDir)) { TxtExportStatus.Text = "输入目录下未找到 TextGrid/ 子目录"; return; }
            if (!File.Exists(_goExePath)) { TxtExportStatus.Text = "未找到 DataForgeLite.exe"; return; }
            TxtExportStatus.Text = "正在导出 transcriptions.csv...";
            BtnExportDataset.IsEnabled = false;
            try
            {
                var exportResult = await Task.Run(() => DataForgeLiteBackend.RunExport(wavsDir, tgDir, outDir, _goExePath));
                var exportObj = exportResult.Output;
                if (exportObj == null || !exportObj.Success)
                {
                    TxtExportStatus.Text = "导出失败: " + (exportObj?.Error ?? exportResult.Stderr ?? "未知错误");
                    return;
                }
                var csvPath = Path.Combine(outDir, "transcriptions.csv");
                if (!string.IsNullOrEmpty(dictPath) && File.Exists(dictPath) && File.Exists(csvPath))
                {
                    TxtExportStatus.Text = "正在计算音素数量...";
                    var addPhResult = await Task.Run(() => DataForgeLiteBackend.RunAddPhNum(csvPath, csvPath, dictPath, "ph_seq", _goExePath));
                    var addPhObj = addPhResult.Output;
                    if (addPhObj == null || !addPhObj.Success)
                        TxtExportStatus.Text = string.Format("导出完成（成功: {0}, 失败: {1}），但 ph_num 计算失败: {2}",
                            exportObj.SuccessCount, exportObj.ErrorCount, addPhObj?.Error ?? addPhResult.Stderr ?? "未知错误");
                    else
                    {
                        TxtExportStatus.Text = string.Format("完成！成功: {0}, 失败: {1}，已写入 ph_num", exportObj.SuccessCount, exportObj.ErrorCount);

                        // 导出后执行 GAME align（Go 实现）
                        if (ChkExportRunGameAlign?.IsChecked == true)
                        {
                            var modelDir = (GetDefaultGameModelPath() ?? "").Trim();
                            var ortDll = (GetDefaultOrtDllPath() ?? "").Trim();
                            var wavDir = Path.Combine(outDir, "wavs");
                            var csvOut2 = Path.Combine(outDir, "transcriptions-midi.csv");

                            if (!Directory.Exists(modelDir))
                            {
                                TxtExportStatus.Text = "已完成 ph_num，但未找到 Gameonnx 模型目录，跳过 GAME Align。";
                                return;
                            }
                            if (!File.Exists(ortDll))
                            {
                                TxtExportStatus.Text = "已完成 ph_num，但未找到 onnxruntime.dll，跳过 GAME Align。";
                                return;
                            }
                            if (!Directory.Exists(wavDir))
                            {
                                TxtExportStatus.Text = "已完成 ph_num，但未找到 wavs/ 目录，跳过 GAME Align。";
                                return;
                            }

                            TxtExportStatus.Text = "正在运行 GAME Align（生成 note_seq/note_dur）...";
                            ApplyOrtDevice();
                            var rr = await Task.Run(() => DataForgeLiteBackend.RunGameAlign(csvPath, csvOut2, wavDir, modelDir, ortDll, "", _goExePath));
                            var goOut = rr.Output;
                            if (goOut != null && goOut.Success)
                            {
                                var hint = string.IsNullOrWhiteSpace(goOut.Error) ? "" : " " + goOut.Error;
                                TxtExportStatus.Text = string.Format("导出完成 + GAME Align：成功 {0} 行，失败 {1} 行。{2}", goOut.SuccessCount, goOut.ErrorCount, hint.Trim());
                            }
                            else
                            {
                                var detail = !string.IsNullOrWhiteSpace(goOut?.Error) ? goOut.Error : (!string.IsNullOrWhiteSpace(rr.Stderr) ? rr.Stderr : "请查看是否 0 行成功或进程异常");
                                TxtExportStatus.Text = string.Format("GAME Align 未全部成功（成功 {0}，失败 {1}）：{2}", goOut?.SuccessCount ?? 0, goOut?.ErrorCount ?? 0, detail);
                            }
                        }
                    }
                }
                else
                {
                    TxtExportStatus.Text = string.Format("导出完成！成功: {0}, 失败: {1}", exportObj.SuccessCount, exportObj.ErrorCount);
                }
                AppState.Instance.AddLog("导出完成: " + exportObj.SuccessCount + " 条");
            }
            catch (Exception ex) { TxtExportStatus.Text = "错误: " + ex.Message; }
            finally { BtnExportDataset.IsEnabled = true; }
        }

        private void BtnBrowseGameAlignCsv1_Click(object sender, RoutedEventArgs e)
        {
            var dlg = new Microsoft.Win32.OpenFileDialog
            {
                Filter = "CSV|*.csv|全部|*.*",
                Multiselect = false,
                Title = "选择 csv1（transcriptions.csv）"
            };
            if (dlg.ShowDialog() == true)
            {
                TxtGameAlignCsv1.Text = dlg.FileName;
                try
                {
                    var dir = System.IO.Path.GetDirectoryName(dlg.FileName);
                    if (!string.IsNullOrWhiteSpace(dir))
                    {
                        var wavs = System.IO.Path.Combine(dir, "wavs");
                        if (Directory.Exists(wavs)) TxtGameAlignWavsDir.Text = wavs;
                        TxtGameAlignCsv2.Text = System.IO.Path.Combine(dir, "transcriptions-midi.csv");
                    }
                }
                catch { }
            }
        }

        private void BtnBrowseGameAlignCsv2_Click(object sender, RoutedEventArgs e)
        {
            var dlg = new Microsoft.Win32.SaveFileDialog
            {
                Filter = "CSV|*.csv|全部|*.*",
                Title = "选择 csv2 保存路径（transcriptions-midi.csv）",
                FileName = "transcriptions-midi.csv"
            };
            if (dlg.ShowDialog() == true) TxtGameAlignCsv2.Text = dlg.FileName;
        }

        private void BtnBrowseGameAlignWavsDir_Click(object sender, RoutedEventArgs e)
        {
            var d = new System.Windows.Forms.FolderBrowserDialog { Description = "选择 wavs 目录（包含 name.wav）", ShowNewFolderButton = false };
            if (d.ShowDialog() == System.Windows.Forms.DialogResult.OK) TxtGameAlignWavsDir.Text = d.SelectedPath;
        }

        private async void BtnRunGameAlignGo_Click(object sender, RoutedEventArgs e)
        {
            var csv1 = (TxtGameAlignCsv1?.Text ?? "").Trim();
            var csv2 = (TxtGameAlignCsv2?.Text ?? "").Trim();
            var wavDir = (TxtGameAlignWavsDir?.Text ?? "").Trim();
            var modelDir = (GetDefaultGameModelPath() ?? "").Trim();
            var ortDll = (GetDefaultOrtDllPath() ?? "").Trim();

            if (string.IsNullOrWhiteSpace(csv1) || !File.Exists(csv1)) { TxtExportStatus.Text = "csv1 不存在"; return; }
            if (string.IsNullOrWhiteSpace(csv2)) { TxtExportStatus.Text = "请填写 csv2 保存路径"; return; }
            if (string.IsNullOrWhiteSpace(wavDir) || !Directory.Exists(wavDir)) { TxtExportStatus.Text = "wavs 目录不存在"; return; }
            if (!Directory.Exists(modelDir)) { TxtExportStatus.Text = "未找到 Gameonnx 模型目录"; return; }
            if (!File.Exists(ortDll)) { TxtExportStatus.Text = "未找到 onnxruntime.dll"; return; }
            if (!File.Exists(_goExePath)) { TxtExportStatus.Text = "未找到 DataForgeLite.exe"; return; }

            BtnRunGameAlignGo.IsEnabled = false;
            TxtExportStatus.Text = "正在运行 GAME Align（Go）...";
            try
            {
                ApplyOrtDevice();
                var rr = await Task.Run(() => DataForgeLiteBackend.RunGameAlign(csv1, csv2, wavDir, modelDir, ortDll, "", _goExePath));
                var outObj = rr.Output;
                if (outObj != null && outObj.Success)
                {
                    var hint = string.IsNullOrWhiteSpace(outObj.Error) ? "" : " " + outObj.Error;
                    TxtExportStatus.Text = string.Format("GAME Align 完成：成功 {0} 行，失败 {1} 行。{2}", outObj.SuccessCount, outObj.ErrorCount, hint.Trim());
                }
                else
                {
                    var detail = !string.IsNullOrWhiteSpace(outObj?.Error) ? outObj.Error : (!string.IsNullOrWhiteSpace(rr.Stderr) ? rr.Stderr : "请检查 CSV/wavs/模型");
                    TxtExportStatus.Text = string.Format("GAME Align 未全部成功（成功 {0}，失败 {1}）：{2}", outObj?.SuccessCount ?? 0, outObj?.ErrorCount ?? 0, detail);
                }
            }
            catch (Exception ex) { TxtExportStatus.Text = "GAME Align 错误: " + ex.Message; }
            finally { BtnRunGameAlignGo.IsEnabled = true; }
        }
    }
}
