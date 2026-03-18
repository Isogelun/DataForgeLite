// Package exporter 提供数据集导出功能
package exporter

import "fmt"

// ExporterError 是导出器的基础错误类型
type ExporterError struct {
	Type    string // 错误类型
	Message string // 错误消息
	Cause   error  // 原始错误
}

// Error 实现 error 接口
func (e *ExporterError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("[%s] %s: %v", e.Type, e.Message, e.Cause)
	}
	return fmt.Sprintf("[%s] %s", e.Type, e.Message)
}

// Unwrap 返回底层错误，支持 errors.Is 和 errors.As
func (e *ExporterError) Unwrap() error {
	return e.Cause
}

// 错误类型常量
const (
	ErrTypeFile   = "FileError"
	ErrTypeParse  = "ParseError"
	ErrTypeConfig = "ConfigError"
)

// NewFileError 创建文件相关错误
func NewFileError(message string, cause error) *ExporterError {
	return &ExporterError{
		Type:    ErrTypeFile,
		Message: message,
		Cause:   cause,
	}
}

// NewParseError 创建解析相关错误
func NewParseError(message string, cause error) *ExporterError {
	return &ExporterError{
		Type:    ErrTypeParse,
		Message: message,
		Cause:   cause,
	}
}

// NewConfigError 创建配置相关错误
func NewConfigError(message string, cause error) *ExporterError {
	return &ExporterError{
		Type:    ErrTypeConfig,
		Message: message,
		Cause:   cause,
	}
}

// IsFileNotFound 检查是否为文件不存在错误
func IsFileNotFound(err error) bool {
	if e, ok := err.(*ExporterError); ok {
		return e.Type == ErrTypeFile
	}
	return false
}

// IsParseError 检查是否为解析错误
func IsParseError(err error) bool {
	if e, ok := err.(*ExporterError); ok {
		return e.Type == ErrTypeParse
	}
	return false
}

// IsConfigError 检查是否为配置错误
func IsConfigError(err error) bool {
	if e, ok := err.(*ExporterError); ok {
		return e.Type == ErrTypeConfig
	}
	return false
}
