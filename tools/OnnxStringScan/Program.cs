using System.Buffers;
using System.Text;

namespace OnnxStringScan;

// Simple ASCII token scanner for binary files (e.g. .onnx).
// It is NOT an ONNX parser; it just helps us quickly discover likely tensor IO names.
internal static class Program
{
    private static bool IsTokenChar(byte b)
    {
        // [A-Za-z0-9_]
        return (b >= (byte)'A' && b <= (byte)'Z')
               || (b >= (byte)'a' && b <= (byte)'z')
               || (b >= (byte)'0' && b <= (byte)'9')
               || b == (byte)'_';
    }

    private static bool IsAsciiLetter(byte b)
    {
        return (b >= (byte)'A' && b <= (byte)'Z') || (b >= (byte)'a' && b <= (byte)'z');
    }

    public static int Main(string[] args)
    {
        if (args.Length < 1)
        {
            Console.Error.WriteLine(
                "Usage: OnnxStringScan <path> [--min N] [--max N] [--top N] [--underscore-only] [--contains SUBSTR]");
            return 2;
        }

        var path = args[0];
        var minLen = 2;
        var maxLen = 64;
        var top = 200;
        var underscoreOnly = false;
        string? contains = null;

        for (var i = 1; i < args.Length; i++)
        {
            switch (args[i])
            {
                case "--min":
                    minLen = int.Parse(args[++i]);
                    break;
                case "--max":
                    maxLen = int.Parse(args[++i]);
                    break;
                case "--top":
                    top = int.Parse(args[++i]);
                    break;
                case "--underscore-only":
                    underscoreOnly = true;
                    break;
                case "--contains":
                    contains = args[++i];
                    break;
                default:
                    Console.Error.WriteLine($"Unknown arg: {args[i]}");
                    return 2;
            }
        }

        if (!File.Exists(path))
        {
            Console.Error.WriteLine($"File not found: {path}");
            return 2;
        }

        var counts = new Dictionary<string, int>(StringComparer.Ordinal);

        // We build tokens in a small byte buffer (ASCII only).
        var tokenBuf = ArrayPool<byte>.Shared.Rent(256);
        var tokenLen = 0;
        var hasUnderscore = false;
        var firstIsLetter = false;

        void FlushToken()
        {
            if (tokenLen < minLen || tokenLen > maxLen || !firstIsLetter)
            {
                tokenLen = 0;
                hasUnderscore = false;
                firstIsLetter = false;
                return;
            }

            if (underscoreOnly && !hasUnderscore)
            {
                tokenLen = 0;
                hasUnderscore = false;
                firstIsLetter = false;
                return;
            }

            var s = Encoding.ASCII.GetString(tokenBuf, 0, tokenLen);
            if (!string.IsNullOrEmpty(contains) &&
                s.IndexOf(contains, StringComparison.OrdinalIgnoreCase) < 0)
            {
                tokenLen = 0;
                hasUnderscore = false;
                firstIsLetter = false;
                return;
            }

            counts.TryGetValue(s, out var c);
            counts[s] = c + 1;

            tokenLen = 0;
            hasUnderscore = false;
            firstIsLetter = false;
        }

        try
        {
            using var fs = File.OpenRead(path);
            var buf = ArrayPool<byte>.Shared.Rent(1 << 20);
            try
            {
                while (true)
                {
                    var n = fs.Read(buf, 0, buf.Length);
                    if (n <= 0) break;

                    for (var i = 0; i < n; i++)
                    {
                        var b = buf[i];
                        if (IsTokenChar(b))
                        {
                            if (tokenLen == 0)
                            {
                                firstIsLetter = IsAsciiLetter(b);
                            }

                            if (tokenLen < 256)
                            {
                                tokenBuf[tokenLen++] = b;
                                if (b == (byte)'_') hasUnderscore = true;
                            }
                            else
                            {
                                // Token is too long/noisy; drop it.
                                tokenLen = 0;
                                hasUnderscore = false;
                                firstIsLetter = false;
                            }
                        }
                        else
                        {
                            if (tokenLen > 0) FlushToken();
                        }
                    }
                }

                if (tokenLen > 0) FlushToken();
            }
            finally
            {
                ArrayPool<byte>.Shared.Return(buf);
            }
        }
        finally
        {
            ArrayPool<byte>.Shared.Return(tokenBuf);
        }

        foreach (var kv in counts.OrderByDescending(kv => kv.Value).ThenBy(kv => kv.Key).Take(top))
        {
            Console.WriteLine($"{kv.Value}\t{kv.Key}");
        }

        return 0;
    }
}
