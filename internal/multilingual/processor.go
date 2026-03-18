package multilingual

// TextProcessor 文本处理器接口
type TextProcessor interface {
	// Process 处理单个文本
	// 参数:
	//   text: 待处理的文本
	// 返回:
	//   string: 转换后的文本
	//   error: 执行错误
	Process(text string) (string, error)

	// GetLanguage 获取处理器支持的语言
	GetLanguage() Language
}

// ProcessorRegistry 处理器注册表
type ProcessorRegistry struct {
	processors map[Language]TextProcessor
	detector   LanguageDetector
}

// NewProcessorRegistry 创建新的处理器注册表
func NewProcessorRegistry() *ProcessorRegistry {
	return &ProcessorRegistry{
		processors: make(map[Language]TextProcessor),
		detector:   NewLanguageDetector(),
	}
}

// RegisterProcessor 注册语言处理器
func (r *ProcessorRegistry) RegisterProcessor(lang Language, processor TextProcessor) {
	r.processors[lang] = processor
}

// GetProcessor 获取指定语言的处理器
func (r *ProcessorRegistry) GetProcessor(lang Language) (TextProcessor, error) {
	processor, exists := r.processors[lang]
	if !exists {
		return nil, ErrProcessorNotRegistered
	}
	return processor, nil
}

// GetDetector 获取语言检测器
func (r *ProcessorRegistry) GetDetector() LanguageDetector {
	return r.detector
}

// Process 处理文本（自动检测语言）
func (r *ProcessorRegistry) Process(text string) (*TextRecord, error) {
	// 检测语言
	lang := r.detector.Detect(text)

	// 如果无法检测语言，原样返回
	if lang == LanguageUnknown {
		return &TextRecord{
			OriginalText:     text,
			DetectedLanguage: LanguageUnknown,
			ConvertedText:    text,
		}, nil
	}

	// 获取对应处理器
	processor, err := r.GetProcessor(lang)
	if err != nil {
		return nil, err
	}

	// 执行转换
	converted, err := processor.Process(text)
	if err != nil {
		return nil, err
	}

	return &TextRecord{
		OriginalText:     text,
		DetectedLanguage: lang,
		ConvertedText:    converted,
	}, nil
}

// ProcessWithLanguage 处理文本（指定语言）
func (r *ProcessorRegistry) ProcessWithLanguage(text string, lang Language) (*TextRecord, error) {
	// 如果是指定语言，直接使用对应处理器
	if lang == LanguageUnknown || lang == "auto" {
		return r.Process(text)
	}

	processor, err := r.GetProcessor(lang)
	if err != nil {
		return nil, err
	}

	converted, err := processor.Process(text)
	if err != nil {
		return nil, err
	}

	return &TextRecord{
		OriginalText:     text,
		DetectedLanguage: lang,
		ConvertedText:    converted,
	}, nil
}

// TextProcessorImpl 文本处理器实现（封装注册表）
type TextProcessorImpl struct {
	registry *ProcessorRegistry
	config   *Config
}

// NewTextProcessor 创建新的文本处理器
func NewTextProcessor(config *Config) (*TextProcessorImpl, error) {
	if config == nil {
		config = DefaultConfig()
	}

	if err := config.Validate(); err != nil {
		return nil, NewError(ErrInvalidConfig, "配置验证失败", err)
	}

	registry := NewProcessorRegistry()

	// 初始化所有处理器
	chineseProcessor := NewChineseProcessor()
	englishProcessor := NewEnglishProcessor()
	japaneseProcessor := NewJapaneseProcessor(config.JapaneseDictPath)

	registry.RegisterProcessor(LanguageChinese, chineseProcessor)
	registry.RegisterProcessor(LanguageEnglish, englishProcessor)
	registry.RegisterProcessor(LanguageJapanese, japaneseProcessor)

	return &TextProcessorImpl{
		registry: registry,
		config:   config,
	}, nil
}

// Process 处理单个文本
func (p *TextProcessorImpl) Process(text string, sourceLanguage string) (*TextRecord, error) {
	if sourceLanguage == "" || sourceLanguage == "auto" {
		return p.registry.Process(text)
	}

	lang := Language(sourceLanguage)
	return p.registry.ProcessWithLanguage(text, lang)
}

// GetDetector 获取语言检测器
func (p *TextProcessorImpl) GetDetector() LanguageDetector {
	return p.registry.GetDetector()
}
