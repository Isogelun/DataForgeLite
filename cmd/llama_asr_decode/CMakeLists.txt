cmake_minimum_required(VERSION 3.14)
project(llama_asr_decode LANGUAGES CXX)

set(CMAKE_CXX_STANDARD 17)
set(CMAKE_CXX_STANDARD_REQUIRED ON)

# LLAMA_BIN_DIR: 预编译 llama.cpp 目录（含 llama.dll 等）
# LLAMA_INCLUDE_DIR: llama.cpp 头文件目录（含 llama.h, ggml.h）
#
# 用法:
#   cmake -B build -DLLAMA_BIN_DIR=../../llama-b8389-bin-win-cpu-x64 -DLLAMA_INCLUDE_DIR=../../llama-b8389-headers
#   cmake --build build --config Release

if(NOT DEFINED LLAMA_BIN_DIR)
    set(LLAMA_BIN_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../../llama-b8389-bin-win-cpu-x64")
endif()

if(NOT DEFINED LLAMA_INCLUDE_DIR)
    set(LLAMA_INCLUDE_DIR "${CMAKE_CURRENT_SOURCE_DIR}/../../llama-b8389-headers")
endif()

# 从 llama.dll 生成导入库（如果 .lib 不存在）
set(LLAMA_DLL "${LLAMA_BIN_DIR}/llama.dll")
set(LLAMA_LIB "${LLAMA_BIN_DIR}/llama.lib")

add_executable(llama_asr_decode main.cpp)

target_include_directories(llama_asr_decode PRIVATE "${LLAMA_INCLUDE_DIR}")

if(EXISTS "${LLAMA_LIB}")
    target_link_libraries(llama_asr_decode PRIVATE "${LLAMA_LIB}")
elseif(MSVC)
    # MSVC: 用 .def 文件生成 .lib
    message(STATUS "llama.lib not found, will attempt to link against llama.dll directly")
    target_link_libraries(llama_asr_decode PRIVATE "${LLAMA_DLL}")
else()
    # MinGW/Clang: 可直接链接 DLL
    target_link_libraries(llama_asr_decode PRIVATE "${LLAMA_DLL}")
endif()

# 构建后复制 exe 到 llama bin 目录（方便 Go 端查找）
add_custom_command(TARGET llama_asr_decode POST_BUILD
    COMMAND ${CMAKE_COMMAND} -E copy_if_different
        $<TARGET_FILE:llama_asr_decode>
        "${LLAMA_BIN_DIR}/llama_asr_decode.exe"
    COMMENT "Copying llama_asr_decode.exe to ${LLAMA_BIN_DIR}"
)
