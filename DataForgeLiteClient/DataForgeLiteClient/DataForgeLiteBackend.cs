using System;
using System.Diagnostics;
using System.IO;
using System.Runtime.Serialization;
using System.Runtime.Serialization.Json;
using System.Text;

namespace DataForgeLiteClient
{
    /// <summary>
    /// 调用 DataForgeLite Go 可执行文件执行 ASR、AddPhNum 等操作。
    /// 约定：Go 端使用 --json 时仅向 stdout 输出一行 JSON，便于解析。
    /// </summary>
    public static class DataForgeLiteBackend
    {
        /// <summary>Go 可执行文件名（Windows）</summary>
        public const string ExeName = "DataForgeLite.exe";

        /// <summary>
        /// 解析 Go 端 --json 输出的统一结构（与 cmd/main.go jsonOutput 对应）
        /// </summary>
        [DataContract]
        public class JsonOutput
        {
            [DataMember(Name = "success")] public bool Success { get; set; }
            [DataMember(Name = "success_count")] public int SuccessCount { get; set; }
            [DataMember(Name = "error_count")] public int ErrorCount { get; set; }
            [DataMember(Name = "error")] public string Error { get; set; }
            [DataMember(Name = "output_path")] public string OutputPath { get; set; }
            [DataMember(Name = "results")] public object Results { get; set; }
        }

        /// <summary>
        /// 查找 DataForgeLite.exe 的路径。
        /// 顺序：当前目录 → 当前目录的父级（项目根）→ 环境变量 DataForgeLiteExe（可选）
        /// </summary>
        public static string GetGoExecutablePath()
        {
            var envPath = Environment.GetEnvironmentVariable("DataForgeLiteExe");
            if (!string.IsNullOrWhiteSpace(envPath) && File.Exists(envPath))
                return envPath;

            var current = Directory.GetCurrentDirectory();
            var inCurrent = Path.Combine(current, ExeName);
            if (File.Exists(inCurrent))
                return inCurrent;

            try
            {
                // 向上查找：同目录、父级、再父级（如 bin\Debug -> DataForgeLiteClient）、再父级（仓库根）
                var dir = Directory.GetParent(current);
                for (int i = 0; i < 4 && dir != null; i++, dir = dir.Parent)
                {
                    var candidate = Path.Combine(dir.FullName, ExeName);
                    if (File.Exists(candidate))
                        return candidate;
                }
            }
            catch
            {
                // 忽略
            }

            return Path.Combine(current, ExeName);
        }

        /// <summary>ASR 调用结果，便于在 Task.Run 中返回</summary>
        public sealed class RunResult
        {
            public JsonOutput Output { get; set; }
            public string StdoutLine { get; set; }
            public string Stderr { get; set; }
        }

        /// <summary>
        /// 运行 ASR：调用 DataForgeLite.exe --asr --input &lt;inputDir&gt; --output &lt;outputDir&gt; [--language &lt;lang&gt;] --backend &lt;backend&gt; --json
        /// </summary>
        /// <param name="language">空=默认中文, "auto"=自动检测, 或 "Chinese"/"English" 等</param>
        /// <param name="backend">"onnx"=纯 Go ONNX（默认）, "python"=Python qwen3asrinfer</param>
        public static RunResult RunAsr(string inputDir, string outputDir, string language, string goExePath, string backend = "onnx")
        {
            var res = new RunResult { Stderr = "" };
            if (string.IsNullOrWhiteSpace(goExePath) || !File.Exists(goExePath))
            {
                res.Stderr = "未找到 DataForgeLite.exe，请先编译 Go 项目并将 exe 放在客户端同目录或上级目录。";
                return res;
            }

            var args = string.Format("--asr --input \"{0}\" --output \"{1}\"",
                (inputDir ?? "").Replace("\"", "\\\""),
                (outputDir ?? "").Replace("\"", "\\\""));
            if (!string.IsNullOrWhiteSpace(language))
                args += string.Format(" --language \"{0}\"", (language ?? "").Replace("\"", "\\\""));
            var backendArg = string.IsNullOrWhiteSpace(backend) ? "onnx" : backend;
            args += string.Format(" --backend {0} --json", backendArg);

            var startInfo = new ProcessStartInfo
            {
                FileName = goExePath,
                Arguments = args,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true,
                StandardOutputEncoding = Encoding.UTF8,
                StandardErrorEncoding = Encoding.UTF8
            };

            try
            {
                using (var process = Process.Start(startInfo))
                {
                    if (process == null)
                    {
                        res.Stderr = "无法启动进程: " + goExePath;
                        return res;
                    }
                    res.StdoutLine = process.StandardOutput.ReadLine();
                    res.Stderr = process.StandardError.ReadToEnd();
                    process.WaitForExit();

                    if (!string.IsNullOrWhiteSpace(res.StdoutLine))
                    {
                        res.Output = ParseJsonOutput(res.StdoutLine);
                        if (res.Output != null)
                            return res;
                    }
                    if (process.ExitCode != 0 && string.IsNullOrWhiteSpace(res.Stderr))
                        res.Stderr = "进程退出码: " + process.ExitCode;
                    return res;
                }
            }
            catch (Exception ex)
            {
                res.Stderr = ex.Message;
                return res;
            }
        }

        /// <summary>
        /// 运行 AddPhNum：DataForgeLite.exe --addphnum --input-csv ... --output-csv ... --dict ... --json
        /// </summary>
        public static RunResult RunAddPhNum(string inputCsv, string outputCsv, string dictPath,
            string phSeqCol, string goExePath)
        {
            var res = new RunResult { Stderr = "" };
            if (string.IsNullOrWhiteSpace(goExePath) || !File.Exists(goExePath))
            {
                res.Stderr = "未找到 DataForgeLite.exe。";
                return res;
            }

            var args = string.Format("--addphnum --input-csv \"{0}\" --output-csv \"{1}\" --dict \"{2}\" --ph-col \"{3}\" --json",
                (inputCsv ?? "").Replace("\"", "\\\""),
                (outputCsv ?? "").Replace("\"", "\\\""),
                (dictPath ?? "").Replace("\"", "\\\""),
                (phSeqCol ?? "ph_seq").Replace("\"", "\\\""));

            var startInfo = new ProcessStartInfo
            {
                FileName = goExePath,
                Arguments = args,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true,
                StandardOutputEncoding = Encoding.UTF8,
                StandardErrorEncoding = Encoding.UTF8
            };

            try
            {
                using (var process = Process.Start(startInfo))
                {
                    if (process == null)
                    {
                        res.Stderr = "无法启动进程: " + goExePath;
                        return res;
                    }
                    res.StdoutLine = process.StandardOutput.ReadLine();
                    res.Stderr = process.StandardError.ReadToEnd();
                    process.WaitForExit();

                    if (!string.IsNullOrWhiteSpace(res.StdoutLine))
                    {
                        res.Output = ParseJsonOutput(res.StdoutLine);
                        if (res.Output != null)
                            return res;
                    }
                    if (process.ExitCode != 0 && string.IsNullOrWhiteSpace(res.Stderr))
                        res.Stderr = "进程退出码: " + process.ExitCode;
                    return res;
                }
            }
            catch (Exception ex)
            {
                res.Stderr = ex.Message;
                return res;
            }
        }

        /// <summary>
        /// 运行预处理：DataForgeLite.exe --preprocess --preprocess-input ... --preprocess-output ... --target-lufs ... --true-peak-limit ... --json
        /// </summary>
        public static RunResult RunPreprocess(string inputDir, string outputDir, double targetLufs, double truePeakLimit, string goExePath)
        {
            return RunGenericJsonPreprocess(goExePath, inputDir, outputDir, targetLufs, truePeakLimit);
        }

        /// <summary>
        /// 运行切分：DataForgeLite.exe --split --split-input ... --split-output ... --json
        /// </summary>
        public static RunResult RunSplit(string inputDir, string outputDir, string goExePath)
        {
            return RunGenericJson(goExePath,
                "--split --split-input \"{0}\" --split-output \"{1}\" --json",
                inputDir, outputDir);
        }

        /// <summary>
        /// 运行导出数据集：DataForgeLite.exe --export --wavs-dir ... --tg-dir ... --export-out ... --json
        /// </summary>
        public static RunResult RunExport(string wavsDir, string tgDir, string exportOutDir, string goExePath)
        {
            var res = new RunResult { Stderr = "" };
            if (string.IsNullOrWhiteSpace(goExePath) || !File.Exists(goExePath))
            {
                res.Stderr = "未找到 DataForgeLite.exe。";
                return res;
            }
            var args = string.Format("--export --wavs-dir \"{0}\" --tg-dir \"{1}\" --export-out \"{2}\" --json",
                (wavsDir ?? "").Replace("\"", "\\\""),
                (tgDir ?? "").Replace("\"", "\\\""),
                (exportOutDir ?? "").Replace("\"", "\\\""));
            var startInfo = new ProcessStartInfo
            {
                FileName = goExePath,
                Arguments = args,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true,
                StandardOutputEncoding = Encoding.UTF8,
                StandardErrorEncoding = Encoding.UTF8
            };
            try
            {
                using (var process = Process.Start(startInfo))
                {
                    if (process == null) { res.Stderr = "无法启动进程"; return res; }
                    res.StdoutLine = process.StandardOutput.ReadLine();
                    res.Stderr = process.StandardError.ReadToEnd();
                    process.WaitForExit();
                    if (!string.IsNullOrWhiteSpace(res.StdoutLine))
                    {
                        res.Output = ParseJsonOutput(res.StdoutLine);
                        if (res.Output != null) return res;
                    }
                    if (process.ExitCode != 0 && string.IsNullOrWhiteSpace(res.Stderr))
                        res.Stderr = "进程退出码: " + process.ExitCode;
                    return res;
                }
            }
            catch (Exception ex) { res.Stderr = ex.Message; return res; }
        }

        /// <summary>
        /// 运行 FA 对齐：DataForgeLite.exe --fa --model-dir ... --input ... --output ... --fa-language ... --non-lexical ... --json
        /// </summary>
        public static RunResult RunFA(string modelDir, string inputDir, string outputDir, string language, string nonLexical, string goExePath)
        {
            var res = new RunResult { Stderr = "" };
            if (string.IsNullOrWhiteSpace(goExePath) || !File.Exists(goExePath))
            {
                res.Stderr = "未找到 DataForgeLite.exe。";
                return res;
            }

            var args = string.Format(
                "--fa --model-dir \"{0}\" --input \"{1}\" --output \"{2}\" --fa-language \"{3}\" --non-lexical \"{4}\" --g2p dictionary --json",
                (modelDir ?? "").Replace("\"", "\\\""),
                (inputDir ?? "").Replace("\"", "\\\""),
                (outputDir ?? "").Replace("\"", "\\\""),
                (language ?? "zh").Replace("\"", "\\\""),
                (nonLexical ?? "AP,EP").Replace("\"", "\\\""));

            var startInfo = new ProcessStartInfo
            {
                FileName = goExePath,
                Arguments = args,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true,
                StandardOutputEncoding = Encoding.UTF8,
                StandardErrorEncoding = Encoding.UTF8
            };

            try
            {
                using (var process = Process.Start(startInfo))
                {
                    if (process == null)
                    {
                        res.Stderr = "无法启动进程: " + goExePath;
                        return res;
                    }
                    res.StdoutLine = process.StandardOutput.ReadLine();
                    res.Stderr = process.StandardError.ReadToEnd();
                    process.WaitForExit();

                    if (!string.IsNullOrWhiteSpace(res.StdoutLine))
                    {
                        res.Output = ParseJsonOutput(res.StdoutLine);
                        if (res.Output != null)
                            return res;
                    }
                    if (process.ExitCode != 0 && string.IsNullOrWhiteSpace(res.Stderr))
                        res.Stderr = "进程退出码: " + process.ExitCode;
                    return res;
                }
            }
            catch (Exception ex)
            {
                res.Stderr = ex.Message;
                return res;
            }
        }

        /// <summary>
        /// 运行 GAME ONNX 推理：
        /// DataForgeLite.exe --game --game-model-dir ... --game-wav ... --game-ort ... --game-lang ... --json
        /// </summary>
        public static RunResult RunGameDir(string modelDir, string inputDir, string ortDllPath, string lang, string outJsonPath, string goExePath)
        {
            var res = new RunResult { Stderr = "" };
            if (string.IsNullOrWhiteSpace(goExePath) || !File.Exists(goExePath))
            {
                res.Stderr = "未找到 DataForgeLite.exe。";
                return res;
            }

            var args = string.Format(
                "--game --game-model-dir \"{0}\" --game-input-dir \"{1}\" --game-ort \"{2}\" --game-lang \"{3}\" --game-out \"{4}\" --json",
                (modelDir ?? "").Replace("\"", "\\\""),
                (inputDir ?? "").Replace("\"", "\\\""),
                (ortDllPath ?? "").Replace("\"", "\\\""),
                (lang ?? "zh").Replace("\"", "\\\""),
                (outJsonPath ?? "").Replace("\"", "\\\"")
            );

            return RunArgs(goExePath, args);
        }

        private static RunResult RunGenericJsonPreprocess(string goExePath, string inputDir, string outputDir, double targetLufs, double truePeakLimit)
        {
            var res = new RunResult { Stderr = "" };
            if (string.IsNullOrWhiteSpace(goExePath) || !File.Exists(goExePath))
            {
                res.Stderr = "未找到 DataForgeLite.exe。";
                return res;
            }
            var args = string.Format(
                "--preprocess --preprocess-input \"{0}\" --preprocess-output \"{1}\" --target-lufs {2} --true-peak-limit {3} --json",
                (inputDir ?? "").Replace("\"", "\\\""),
                (outputDir ?? "").Replace("\"", "\\\""),
                targetLufs.ToString("g", System.Globalization.CultureInfo.InvariantCulture),
                truePeakLimit.ToString("g", System.Globalization.CultureInfo.InvariantCulture));
            var startInfo = new ProcessStartInfo
            {
                FileName = goExePath,
                Arguments = args,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true,
                StandardOutputEncoding = Encoding.UTF8,
                StandardErrorEncoding = Encoding.UTF8
            };
            try
            {
                using (var process = Process.Start(startInfo))
                {
                    if (process == null) { res.Stderr = "无法启动进程"; return res; }
                    res.StdoutLine = process.StandardOutput.ReadLine();
                    res.Stderr = process.StandardError.ReadToEnd();
                    process.WaitForExit();
                    if (!string.IsNullOrWhiteSpace(res.StdoutLine))
                    {
                        res.Output = ParseJsonOutput(res.StdoutLine);
                        if (res.Output != null) return res;
                    }
                    if (process.ExitCode != 0 && string.IsNullOrWhiteSpace(res.Stderr))
                        res.Stderr = "进程退出码: " + process.ExitCode;
                    return res;
                }
            }
            catch (Exception ex) { res.Stderr = ex.Message; return res; }
        }

        private static RunResult RunGenericJson(string goExePath, string argsFormat, string arg0, string arg1)
        {
            var res = new RunResult { Stderr = "" };
            if (string.IsNullOrWhiteSpace(goExePath) || !File.Exists(goExePath))
            {
                res.Stderr = "未找到 DataForgeLite.exe。";
                return res;
            }
            var args = string.Format(argsFormat,
                (arg0 ?? "").Replace("\"", "\\\""),
                (arg1 ?? "").Replace("\"", "\\\""));
            var startInfo = new ProcessStartInfo
            {
                FileName = goExePath,
                Arguments = args,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true,
                StandardOutputEncoding = Encoding.UTF8,
                StandardErrorEncoding = Encoding.UTF8
            };
            try
            {
                using (var process = Process.Start(startInfo))
                {
                    if (process == null) { res.Stderr = "无法启动进程"; return res; }
                    res.StdoutLine = process.StandardOutput.ReadLine();
                    res.Stderr = process.StandardError.ReadToEnd();
                    process.WaitForExit();
                    if (!string.IsNullOrWhiteSpace(res.StdoutLine))
                    {
                        res.Output = ParseJsonOutput(res.StdoutLine);
                        if (res.Output != null) return res;
                    }
                    if (process.ExitCode != 0 && string.IsNullOrWhiteSpace(res.Stderr))
                        res.Stderr = "进程退出码: " + process.ExitCode;
                    return res;
                }
            }
            catch (Exception ex) { res.Stderr = ex.Message; return res; }
        }

        /// <summary>
        /// 运行合并标注：DataForgeLite.exe --combine --combine-wavs ... --combine-tg ... --combine-out ... --combine-suffix ... [--combine-overwrite] --json
        /// </summary>
        public static RunResult RunCombine(string wavsDir, string tgDir, string outDir, string suffix, bool overwrite, string goExePath)
        {
            var res = new RunResult { Stderr = "" };
            if (string.IsNullOrWhiteSpace(goExePath) || !File.Exists(goExePath))
            {
                res.Stderr = "未找到 DataForgeLite.exe。";
                return res;
            }
            var args = string.Format("--combine --combine-wavs \"{0}\" --combine-out \"{1}\" --combine-suffix \"{2}\" --json",
                (wavsDir ?? "").Replace("\"", "\\\""),
                (outDir ?? "").Replace("\"", "\\\""),
                (suffix ?? @"_\d+").Replace("\"", "\\\""));
            if (!string.IsNullOrWhiteSpace(tgDir))
                args = string.Format("--combine --combine-wavs \"{0}\" --combine-tg \"{1}\" --combine-out \"{2}\" --combine-suffix \"{3}\" --json",
                    (wavsDir ?? "").Replace("\"", "\\\""),
                    tgDir.Replace("\"", "\\\""),
                    (outDir ?? "").Replace("\"", "\\\""),
                    (suffix ?? @"_\d+").Replace("\"", "\\\""));
            if (overwrite) args += " --combine-overwrite";
            return RunArgs(goExePath, args);
        }

        public static RunResult RunSliceTg(string inDir, string outDir, int digits, bool preserveName, bool overwrite, string goExePath)
        {
            var res = new RunResult { Stderr = "" };
            if (string.IsNullOrWhiteSpace(goExePath) || !File.Exists(goExePath))
            {
                res.Stderr = "未找到 DataForgeLite.exe。";
                return res;
            }
            var args = string.Format("--slice-tg --slice-in \"{0}\" --slice-out \"{1}\" --slice-digits {2} --json",
                (inDir ?? "").Replace("\"", "\\\""),
                (outDir ?? "").Replace("\"", "\\\""),
                digits);
            if (preserveName) args += " --slice-preserve-name";
            if (overwrite) args += " --slice-overwrite";
            return RunArgs(goExePath, args);
        }

        private static RunResult RunArgs(string goExePath, string args)
        {
            var res = new RunResult { Stderr = "" };
            var startInfo = new ProcessStartInfo
            {
                FileName = goExePath,
                Arguments = args,
                UseShellExecute = false,
                RedirectStandardOutput = true,
                RedirectStandardError = true,
                CreateNoWindow = true,
                StandardOutputEncoding = Encoding.UTF8,
                StandardErrorEncoding = Encoding.UTF8
            };
            try
            {
                using (var process = Process.Start(startInfo))
                {
                    if (process == null) { res.Stderr = "无法启动进程"; return res; }
                    res.StdoutLine = process.StandardOutput.ReadLine();
                    res.Stderr = process.StandardError.ReadToEnd();
                    process.WaitForExit();
                    if (!string.IsNullOrWhiteSpace(res.StdoutLine))
                    {
                        res.Output = ParseJsonOutput(res.StdoutLine);
                        if (res.Output != null) return res;
                    }
                    if (process.ExitCode != 0 && string.IsNullOrWhiteSpace(res.Stderr))
                        res.Stderr = "进程退出码: " + process.ExitCode;
                    return res;
                }
            }
            catch (Exception ex) { res.Stderr = ex.Message; return res; }
        }

        private static JsonOutput ParseJsonOutput(string jsonLine)
        {
            if (string.IsNullOrWhiteSpace(jsonLine)) return null;
            try
            {
                var ms = new MemoryStream(Encoding.UTF8.GetBytes(jsonLine.Trim()));
                var ser = new DataContractJsonSerializer(typeof(JsonOutput));
                return (JsonOutput)ser.ReadObject(ms);
            }
            catch
            {
                return null;
            }
        }

        /// <summary>
        /// 运行 GAME align（Go 实现）：写回 note_seq/note_dur。
        /// lang 为空时与 infer.py align 未加 -l 一致（language_id=0）；需与 Python 一致时不要随便填 zh。
        /// </summary>
        public static RunResult RunGameAlign(string csv1, string csv2, string wavDir, string modelDir, string ortDllPath, string lang, string goExePath)
        {
            var res = new RunResult { Stderr = "" };
            if (string.IsNullOrWhiteSpace(goExePath) || !File.Exists(goExePath))
            {
                res.Stderr = "未找到 DataForgeLite.exe。";
                return res;
            }

            var l = (lang ?? "").Replace("\"", "\\\"");
            var langArg = string.IsNullOrWhiteSpace(lang)
                ? ""
                : string.Format(" --game-align-lang \"{0}\"", l);
            var args = string.Format(
                "--game-align --game-align-in \"{0}\" --game-align-out \"{1}\" --game-align-wavs \"{2}\" --game-model-dir \"{3}\" --game-ort \"{4}\"{5} --json",
                (csv1 ?? "").Replace("\"", "\\\""),
                (csv2 ?? "").Replace("\"", "\\\""),
                (wavDir ?? "").Replace("\"", "\\\""),
                (modelDir ?? "").Replace("\"", "\\\""),
                (ortDllPath ?? "").Replace("\"", "\\\""),
                langArg
            );

            return RunArgs(goExePath, args);
        }
    }
}
