using System;
using System.Text;
using System.Text.RegularExpressions;

namespace DataForgeLiteClient
{
    /// <summary>
    /// 汉字转无标点、无声调拼音（与 ASR 的 .lab / _pinyin.txt 格式一致）。
    /// 使用 NPinyin.Core 时去除声调数字；无包时做简单占位。
    /// </summary>
    public static class PinyinHelper
    {
        private static readonly Regex ToneDigits = new Regex(@"[1-5]", RegexOptions.Compiled);

        /// <summary>
        /// 将中文文本转为无标点、无声调、空格分隔的拼音。
        /// 使用 NPinyin.Core 时去除声调；未引用时仅做去标点与空白规范化（保留汉字）。
        /// </summary>
        public static string ToPinyinNoTone(string text)
        {
            if (string.IsNullOrEmpty(text)) return "";
            string pinyin = null;
            try
            {
                var type = Type.GetType("NPinyin.Pinyin, NPinyin.Core");
                if (type != null)
                {
                    var method = type.GetMethod("GetPinyin", new[] { typeof(string) });
                    if (method != null)
                        pinyin = method.Invoke(null, new object[] { text }) as string;
                }
            }
            catch { /* 无 NPinyin.Core 时忽略 */ }
            if (!string.IsNullOrEmpty(pinyin))
            {
                pinyin = RemoveToneMarks(pinyin);
                pinyin = ToneDigits.Replace(pinyin, "");
                pinyin = NormalizeSpaces(pinyin);
                return pinyin.Trim();
            }
            return SimpleFallback(text);
        }

        private static string RemoveToneMarks(string s)
        {
            var n = new StringBuilder(s.Length);
            foreach (var c in s)
            {
                var r = RemoveTone(c);
                n.Append(r);
            }
            return n.ToString();
        }

        private static char RemoveTone(char c)
        {
            switch (c)
            {
                case 'ā': case 'á': case 'ǎ': case 'à': return 'a';
                case 'ē': case 'é': case 'ě': case 'è': return 'e';
                case 'ī': case 'í': case 'ǐ': case 'ì': return 'i';
                case 'ō': case 'ó': case 'ǒ': case 'ò': return 'o';
                case 'ū': case 'ú': case 'ǔ': case 'ù': return 'u';
                case 'ǖ': case 'ǘ': case 'ǚ': case 'ǜ': return 'v'; // ü → v 与 pypinyin 一致
                case 'ń': case 'ň': case 'ǹ': return 'n';
                case 'ḿ': return 'm';
                default: return c;
            }
        }

        private static string NormalizeSpaces(string s)
        {
            return Regex.Replace(s.Trim(), @"\s+", " ");
        }

        private static string SimpleFallback(string text)
        {
            var sb = new StringBuilder();
            foreach (var c in text)
            {
                if (char.IsWhiteSpace(c))
                    sb.Append(' ');
                else if (IsCjk(c))
                    sb.Append(c); // 无字典时保留汉字
                else if (!char.IsPunctuation(c) && !char.IsSymbol(c))
                    sb.Append(c);
            }
            return NormalizeSpaces(sb.ToString());
        }

        private static bool IsCjk(char c)
        {
            var u = (uint)c;
            return (u >= 0x4E00 && u <= 0x9FFF) || (u >= 0x3400 && u <= 0x4DBF);
        }
    }
}
