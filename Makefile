# DataForgeLite Makefile

.PHONY: help build build-dev test clean fmt lint setup-dev run deps-update

# 默认目标
help:
	@echo "DataForgeLite 可用命令:"
	@echo ""
	@echo "  build        构建所有平台的发布版本"
	@echo "  build-dev    构建开发版本（当前平台）"
	@echo "  test         运行测试"
	@echo "  clean        清理构建文件"
	@echo "  fmt          格式化代码"
	@echo "  lint         代码检查"
	@echo "  setup-dev    设置开发环境"
	@echo "  run          运行开发版本"
	@echo "  deps-update  更新依赖"
	@echo ""

# 变量定义
APP_NAME := dataforgether
VERSION := $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
BUILD_TIME := $(shell date +%Y-%m-%d_%H:%M:%S)
LDFLAGS := -ldflags "-X main.version=$(VERSION) -X main.buildTime=$(BUILD_TIME) -s -w"

# 构建所有平台的发布版本
build:
	@echo "构建所有平台的发布版本..."
	@mkdir -p dist
	@CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o dist/$(APP_NAME)-windows-amd64.exe ./cmd/dataforgether
	@CGO_ENABLED=0 GOOS=windows GOARCH=arm64 go build $(LDFLAGS) -o dist/$(APP_NAME)-windows-arm64.exe ./cmd/dataforgether
	@CGO_ENABLED=0 GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o dist/$(APP_NAME)-darwin-amd64 ./cmd/dataforgether
	@CGO_ENABLED=0 GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o dist/$(APP_NAME)-darwin-arm64 ./cmd/dataforgether
	@CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o dist/$(APP_NAME)-linux-amd64 ./cmd/dataforgether
	@CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o dist/$(APP_NAME)-linux-arm64 ./cmd/dataforgether
	@echo "构建完成! 文件位于 dist/ 目录"

# 构建开发版本（当前平台）
build-dev:
	@echo "构建开发版本..."
	@mkdir -p bin
	go build $(LDFLAGS) -o bin/$(APP_NAME) ./cmd/dataforgether
	@echo "开发版本构建完成! 文件位于 bin/$(APP_NAME)"

# 运行测试
test:
	@echo "运行测试..."
	go test -v ./...

# 清理构建文件
clean:
	@echo "清理构建文件..."
	@rm -rf bin/ dist/
	@echo "清理完成"

# 格式化代码
fmt:
	@echo "格式化代码..."
	go fmt ./...
	@echo "格式化完成"

# 代码检查
lint:
	@echo "代码检查..."
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run; \
	else \
		echo "golangci-lint 未安装，请先安装: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest"; \
	fi

# 设置开发环境
setup-dev:
	@echo "设置开发环境..."
	@go version
	@go mod tidy
	@go mod download
	@if command -v golangci-lint >/dev/null 2>&1; then \
		echo "golangci-lint 已安装"; \
	else \
		echo "安装golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
	fi
	@echo "开发环境设置完成"

# 运行开发版本
run: build-dev
	@echo "运行开发版本..."
	./bin/$(APP_NAME)

# 更新依赖
deps-update:
	@echo "更新依赖..."
	go get -u ./...
	go mod tidy
	@echo "依赖更新完成"

# 生成文档
docs:
	@echo "生成文档..."
	@if command -v godoc >/dev/null 2>&1; then \
		echo "启动文档服务器: http://localhost:6060/pkg/github.com/dataforge/dataforgether/"; \
		godoc -http=:6060; \
	else \
		echo "godoc 未安装，请先安装: go install golang.org/x/tools/cmd/godoc@latest"; \
	fi

# 安装依赖工具
install-tools:
	@echo "安装开发工具..."
	@go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	@go install golang.org/x/tools/cmd/godoc@latest
	@go install github.com/air-verse/air@latest
	@echo "开发工具安装完成"

# 热重载开发（需要先安装air）
watch:
	@echo "启动热重载开发..."
	@if command -v air >/dev/null 2>&1; then \
		air; \
	else \
		echo "air 未安装，请先运行 make install-tools"; \
	fi

# 打包发布
package: build
	@echo "打包发布版本..."
	@mkdir -p packages
	@cd dist && \
	for file in *; do \
		if [ -f "$$file" ]; then \
			extension="$${file##*.}"; \
			if [ "$$extension" = "exe" ]; then \
				zip -r "../packages/$${file%.*}.zip" "$$file" ../assets/; \
			else \
				tar -czf "../packages/$$file.tar.gz" "$$file" ../assets/; \
			fi; \
		fi; \
	done
	@echo "打包完成! 文件位于 packages/ 目录"

# 性能测试
bench:
	@echo "运行性能测试..."
	go test -bench=. -benchmem ./...

# 覆盖率测试
coverage:
	@echo "运行覆盖率测试..."
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "覆盖率报告已生成: coverage.html"

# 检查更新
check-update:
	@echo "检查依赖更新..."
	go list -u -m all