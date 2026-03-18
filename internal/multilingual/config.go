package multilingual

// Config 多语言文本处理器配置
type Config struct {
	// ChineseDictPath 中文拼音词典路径
	ChineseDictPath string
	// JapaneseDictPath 日文罗马音词典路径
	JapaneseDictPath string
	// DefaultLanguage 默认语言（auto/zh/en/ja）
	DefaultLanguage string
	// OutputFormat 输出格式配置
	OutputFormat OutputFormat
}

// OutputFormat 输出格式配置
type OutputFormat struct {
	// Lowercase 是否转换为小写
	Lowercase bool
	// Separator 分隔符，默认空格
	Separator string
}

// DefaultConfig 返回默认配置
func DefaultConfig() *Config {
	return &Config{
		DefaultLanguage: "auto",
		OutputFormat: OutputFormat{
			Lowercase: true,
			Separator: " ",
		},
	}
}

// Validate 验证配置有效性
func (c *Config) Validate() error {
	if c.DefaultLanguage == "" {
		c.DefaultLanguage = "auto"
	}
	if c.OutputFormat.Separator == "" {
		c.OutputFormat.Separator = " "
	}
	return nil
}
