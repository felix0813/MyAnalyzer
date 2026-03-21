#!/bin/bash

# 简单构建脚本
BUILD_DIR="./build"
mkdir -p $BUILD_DIR

echo "构建 myanalyzer 服务器..."

# 构建 Linux 版本
GOOS=linux GOARCH=amd64 go build -o $BUILD_DIR/myanalyzer-server-linux-amd64 ./cmd/server/

# 构建 Windows 版本  
# GOOS=windows GOARCH=amd64 go build -o $BUILD_DIR/myanalyzer-server-windows-amd64.exe ./cmd/server/

# 构建 macOS 版本
# GOOS=darwin GOARCH=amd64 go build -o $BUILD_DIR/myanalyzer-server-darwin-amd64 ./cmd/server/

echo "构建完成，文件保存在 $BUILD_DIR 目录中"