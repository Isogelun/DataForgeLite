/**
 * llama_asr_decode: 读取 DataForgeLite Go 写入的 embedding 文件，用 llama.cpp 解码为文本。
 *
 * 嵌入文件格式：4 字节 seqLen (int32), 4 字节 dim (int32), 再 seqLen*dim 个 float32。
 *
 * 构建 prompt（与 Go tokenizer.go 一致）：
 *   <|im_start|>system\nYou are a helpful assistant.<|im_end|>\n
 *   <|im_start|>user\n<|audio_start|>{audio_emb x seqLen}<|audio_end|><|im_end|>\n
 *   <|im_start|>assistant\n<|asr_text|>
 *
 * 分三段 prefill：
 *   1) prefix tokens (up to and including <|audio_start|>) via batch.token
 *   2) audio embeddings via batch.embd
 *   3) suffix tokens (<|audio_end|> onward) via batch.token
 * 然后自回归采样直到 EOS。
 */
#include "llama.h"

#include <cstdint>
#include <cstdio>
#include <cstdlib>
#include <cstring>
#include <fstream>
#include <string>
#include <vector>

// Qwen3-ASR special token IDs (must match Go tokenizer.go)
static constexpr llama_token TOK_IM_START    = 151644;
static constexpr llama_token TOK_IM_END      = 151645;
static constexpr llama_token TOK_END_OF_TEXT = 151643;
static constexpr llama_token TOK_AUDIO_START = 151669;
static constexpr llama_token TOK_AUDIO_END   = 151670;
static constexpr llama_token TOK_ASR_TEXT    = 151704;

// --- helpers ---

static bool load_embeddings(const char* path, int32_t& rows, int32_t& dim, std::vector<float>& data) {
    std::ifstream f(path, std::ios::binary);
    if (!f) return false;
    f.read(reinterpret_cast<char*>(&rows), sizeof(rows));
    f.read(reinterpret_cast<char*>(&dim), sizeof(dim));
    if (rows <= 0 || dim <= 0 || rows > 100000 || dim > 10000) return false;
    size_t n = (size_t)rows * (size_t)dim;
    data.resize(n);
    f.read(reinterpret_cast<char*>(data.data()), n * sizeof(float));
    return f.good();
}

static bool is_eos(llama_token id) {
    return id == TOK_END_OF_TEXT || id == TOK_IM_END;
}

static std::vector<llama_token> tokenize(const llama_vocab* vocab, const char* text) {
    std::vector<llama_token> ids;
    int n = llama_tokenize(vocab, text, (int)strlen(text), nullptr, 0, false, true);
    if (n < 0) {
        ids.resize(-n);
        llama_tokenize(vocab, text, (int)strlen(text), ids.data(), (int)ids.size(), false, true);
    }
    return ids;
}

// Build prefix tokens: everything up to and including <|audio_start|>
static std::vector<llama_token> build_prefix(const llama_vocab* vocab) {
    std::vector<llama_token> ids;
    ids.push_back(TOK_IM_START);
    auto t = tokenize(vocab, "system\nYou are a helpful assistant.");
    ids.insert(ids.end(), t.begin(), t.end());
    ids.push_back(TOK_IM_END);
    t = tokenize(vocab, "\n");
    ids.insert(ids.end(), t.begin(), t.end());
    ids.push_back(TOK_IM_START);
    t = tokenize(vocab, "user\n");
    ids.insert(ids.end(), t.begin(), t.end());
    ids.push_back(TOK_AUDIO_START);
    return ids;
}

// Build suffix tokens: <|audio_end|><|im_end|>\n<|im_start|>assistant\n<|asr_text|>
static std::vector<llama_token> build_suffix(const llama_vocab* vocab) {
    std::vector<llama_token> ids;
    ids.push_back(TOK_AUDIO_END);
    ids.push_back(TOK_IM_END);
    auto t = tokenize(vocab, "\n");
    ids.insert(ids.end(), t.begin(), t.end());
    ids.push_back(TOK_IM_START);
    t = tokenize(vocab, "assistant\n");
    ids.insert(ids.end(), t.begin(), t.end());
    ids.push_back(TOK_ASR_TEXT);
    return ids;
}

// Decode a batch of tokens (batch.token mode). Returns 0 on success.
static int decode_tokens(llama_context* ctx, const llama_token* tokens, int n,
                         llama_pos pos0, bool logits_last) {
    llama_batch batch = llama_batch_init(n, 0, 1);
    batch.n_tokens = n;
    for (int i = 0; i < n; i++) {
        batch.token[i]    = tokens[i];
        batch.pos[i]      = pos0 + i;
        batch.n_seq_id[i] = 1;
        batch.seq_id[i][0] = 0;
        batch.logits[i]   = (logits_last && i == n - 1) ? 1 : 0;
    }
    int rc = llama_decode(ctx, batch);
    llama_batch_free(batch);
    return rc;
}

// Decode a batch of embeddings (batch.embd mode). Returns 0 on success.
static int decode_embeddings(llama_context* ctx, const float* embd, int n, int dim,
                             llama_pos pos0, bool logits_last) {
    llama_batch batch = llama_batch_init(n, dim, 1);
    batch.n_tokens = n;
    memcpy(batch.embd, embd, (size_t)n * dim * sizeof(float));
    for (int i = 0; i < n; i++) {
        batch.pos[i]      = pos0 + i;
        batch.n_seq_id[i] = 1;
        batch.seq_id[i][0] = 0;
        batch.logits[i]   = (logits_last && i == n - 1) ? 1 : 0;
    }
    int rc = llama_decode(ctx, batch);
    llama_batch_free(batch);
    return rc;
}

static void usage(const char* prog) {
    fprintf(stderr, "用法: %s --model <模型目录> --embeddings <.bin文件> [--max-tokens 256]\n", prog);
}

int main(int argc, char** argv) {
    const char* model_dir = nullptr;
    const char* emb_path  = nullptr;
    int max_tokens = 256;

    for (int i = 1; i < argc; i++) {
        if (strcmp(argv[i], "--model") == 0 && i + 1 < argc)      { model_dir = argv[++i]; continue; }
        if (strcmp(argv[i], "--embeddings") == 0 && i + 1 < argc)  { emb_path  = argv[++i]; continue; }
        if (strcmp(argv[i], "--max-tokens") == 0 && i + 1 < argc)  { max_tokens = atoi(argv[++i]); continue; }
    }
    if (!model_dir || !emb_path) {
        usage(argv[0]);
        return 1;
    }

    // 1. Load audio embeddings from file
    int32_t audio_rows = 0, audio_dim = 0;
    std::vector<float> audio_emb;
    if (!load_embeddings(emb_path, audio_rows, audio_dim, audio_emb)) {
        fprintf(stderr, "无法读取嵌入文件: %s\n", emb_path);
        return 1;
    }

    // 2. Locate GGUF model file
    std::string gguf_path = std::string(model_dir) + "/qwen3_asr_llm.q4_k.gguf";
    {
        std::ifstream test(gguf_path);
        if (!test.good()) {
            gguf_path = std::string(model_dir) + "\\qwen3_asr_llm.q4_k.gguf";
        }
    }

    // 3. Load model
    llama_model_params mparams = llama_model_default_params();
    mparams.n_gpu_layers = 0;
    llama_model* model = llama_model_load_from_file(gguf_path.c_str(), mparams);
    if (!model) {
        fprintf(stderr, "无法加载模型: %s\n", gguf_path.c_str());
        return 1;
    }

    const llama_vocab* vocab = llama_model_get_vocab(model);
    int model_dim = llama_model_n_embd(model);

    // 4. Build prefix / suffix token sequences
    auto prefix = build_prefix(vocab);
    auto suffix = build_suffix(vocab);
    int n_total = (int)prefix.size() + audio_rows + (int)suffix.size();

    // 5. Create context
    llama_context_params cparams = llama_context_default_params();
    cparams.n_ctx   = n_total + max_tokens + 64;
    cparams.n_batch = n_total;
    llama_context* ctx = llama_init_from_model(model, cparams);
    if (!ctx) {
        fprintf(stderr, "无法创建 llama context\n");
        llama_model_free(model);
        return 1;
    }

    // 6. Three-stage prefill
    llama_pos pos = 0;

    // Stage 1: prefix tokens
    if (decode_tokens(ctx, prefix.data(), (int)prefix.size(), pos, false) != 0) {
        fprintf(stderr, "prefill prefix 失败\n");
        llama_free(ctx); llama_model_free(model);
        return 1;
    }
    pos += (int)prefix.size();

    // Stage 2: audio embeddings
    // If audio_dim != model_dim, pad or truncate each row
    std::vector<float> audio_adj;
    const float* audio_ptr = audio_emb.data();
    if (audio_dim != model_dim) {
        audio_adj.resize((size_t)audio_rows * model_dim, 0.0f);
        int cp = (audio_dim < model_dim) ? audio_dim : model_dim;
        for (int r = 0; r < audio_rows; r++) {
            memcpy(audio_adj.data() + r * model_dim,
                   audio_emb.data() + r * audio_dim,
                   cp * sizeof(float));
        }
        audio_ptr = audio_adj.data();
    }

    // Feed audio embeddings in chunks to stay within n_batch
    {
        int batch_size = n_total; // context n_batch is set to n_total
        int sent = 0;
        while (sent < audio_rows) {
            int chunk = audio_rows - sent;
            if (chunk > batch_size) chunk = batch_size;
            bool last_chunk = (sent + chunk == audio_rows) && suffix.empty();
            if (decode_embeddings(ctx, audio_ptr + (size_t)sent * model_dim,
                                  chunk, model_dim, pos, last_chunk) != 0) {
                fprintf(stderr, "prefill audio embeddings 失败 (offset %d)\n", sent);
                llama_free(ctx); llama_model_free(model);
                return 1;
            }
            pos += chunk;
            sent += chunk;
        }
    }

    // Stage 3: suffix tokens (need logits on last token for sampling)
    if (decode_tokens(ctx, suffix.data(), (int)suffix.size(), pos, true) != 0) {
        fprintf(stderr, "prefill suffix 失败\n");
        llama_free(ctx); llama_model_free(model);
        return 1;
    }
    pos += (int)suffix.size();

    // 7. Autoregressive decoding with greedy sampling
    {
        llama_sampler* smpl = llama_sampler_chain_init(llama_sampler_chain_default_params());
        llama_sampler_chain_add(smpl, llama_sampler_init_greedy());

        std::string output_text;

        for (int i = 0; i < max_tokens; i++) {
            llama_token new_token = llama_sampler_sample(smpl, ctx, -1);

            if (is_eos(new_token)) break;

            char buf[256];
            int n = llama_token_to_piece(vocab, new_token, buf, sizeof(buf), 0, true);
            if (n > 0) output_text.append(buf, n);

            // Feed new token for next step
            llama_batch batch = llama_batch_init(1, 0, 1);
            batch.n_tokens    = 1;
            batch.token[0]    = new_token;
            batch.pos[0]      = pos;
            batch.n_seq_id[0] = 1;
            batch.seq_id[0][0] = 0;
            batch.logits[0]   = 1;

            if (llama_decode(ctx, batch) != 0) {
                fprintf(stderr, "decode step %d 失败\n", i);
                llama_batch_free(batch);
                break;
            }
            llama_batch_free(batch);
            pos++;
        }

        llama_sampler_free(smpl);
        fprintf(stdout, "%s", output_text.c_str());
    }

    llama_free(ctx);
    llama_model_free(model);
    return 0;
}
