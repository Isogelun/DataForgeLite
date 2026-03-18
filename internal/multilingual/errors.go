package multilingual

import (
	"fmt"
)

// ErrorCode 错误码类型
type ErrorCode string

const (
	// ErrLanguageDetect 语言检测失败
	ErrLanguageDetect ErrorCode = "LANG_DETECT_FAILED"
	// ErrProcessorNotFound 处理器未找到
	ErrProcessorNotFound ErrorCode = "PROCESSOR_NOT_FOUND"
	// ErrDictionaryLoad 词典加载失败
	ErrDictionaryLoad ErrorCode = "DICT_LOAD_FAILED"
	// ErrConversion 转换失败
	ErrConversion ErrorCode = "CONVERSION_FAILED"
	// ErrInvalidConfig 配置无效
	ErrInvalidConfig ErrorCode = "INVALID_CONFIG"
	// ErrFileOperation 文件操作失败
	ErrFileOperation ErrorCode = "FILE_OPERATION_FAILED"
)

// Error 多语言文本处理错误
type Error struct {
	Code    ErrorCode
	Message string
	Cause   error
}

// Error 实现 error 接口
func (e *Error) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Code, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Code, e.Message)
}

// Unwrap 返回底层错误
func (e *Error) Unwrap() error {
	return e.Cause
}

// NewError 创建新的错误
func NewError(code ErrorCode, message string, cause error) *Error {
	return &Error{
		Code:    code,
		Message: message,
		Cause:   cause,
	}
}

// 预定义错误
var (
	// ErrDictionaryNotFound 词典文件不存在
	ErrDictionaryNotFound = NewError(ErrDictionaryLoad, "词典文件不存在", nil)
	// ErrInvalidLanguage 无效的语言类型
	ErrInvalidLanguage = NewError(ErrLanguageDetect, "无效的语言类型", nil)
	// ErrProcessorNotRegistered 处理器未注册
	ErrProcessorNotRegistered = NewError(ErrProcessorNotFound, "处理器未注册", nil)
)
