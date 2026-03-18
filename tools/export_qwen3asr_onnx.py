"""
One-time script to export Qwen3-ASR-0.6B to ONNX format.

Produces three files:
  - encoder.onnx        : audio features -> encoder hidden states
  - decoder_init.onnx   : first step (no past key values)
  - decoder.onnx        : subsequent steps (with past key values)

Usage:
  pip install qwen_asr torch onnx onnxruntime
  python export_qwen3asr_onnx.py --model-dir ../qwen3asrinfer/Qwen3-ASR-0.6B --output-dir ../qwen3asrinfer/Qwen3-ASR-0.6B/onnx

The ONNX files are written into the output-dir.
"""

import argparse
import os
import sys
import torch
import torch.nn as nn
from pathlib import Path


def get_model_and_processor(model_dir: str):
    """Load the Qwen3ASR model and processor from a local directory."""
    try:
        from qwen_asr import Qwen3ASRForConditionalGeneration, Qwen3ASRProcessor
    except ImportError:
        print("ERROR: qwen_asr package not found. Install with: pip install qwen_asr")
        sys.exit(1)

    print(f"Loading model from {model_dir} ...")
    processor = Qwen3ASRProcessor.from_pretrained(model_dir)
    model = Qwen3ASRForConditionalGeneration.from_pretrained(
        model_dir, torch_dtype=torch.float32
    )
    model.eval()
    return model, processor


class AudioEncoderWrapper(nn.Module):
    """Wraps the audio encoder for ONNX export.
    Input:  mel_features [batch, 128, T_mel]
    Output: encoder_hidden_states [batch, seq_len, hidden_size]
    """

    def __init__(self, model):
        super().__init__()
        self.audio_encoder = model.thinker.audio_encoder
        self.multi_modal_projector = model.thinker.multi_modal_projector

    def forward(self, mel_features):
        encoder_out = self.audio_encoder(mel_features)
        # encoder_out may be a tuple or have .last_hidden_state
        if hasattr(encoder_out, "last_hidden_state"):
            hidden = encoder_out.last_hidden_state
        elif isinstance(encoder_out, tuple):
            hidden = encoder_out[0]
        else:
            hidden = encoder_out
        projected = self.multi_modal_projector(hidden)
        return projected


class DecoderInitWrapper(nn.Module):
    """Wraps the text decoder for the first step (no past_key_values).
    Input:  input_ids [batch, seq_len], encoder_hidden_states [batch, enc_len, hidden]
    Output: logits [batch, seq_len, vocab_size], present_key_values (flattened)
    """

    def __init__(self, model):
        super().__init__()
        self.text_model = model.thinker.text_model
        self.lm_head = model.lm_head if hasattr(model, "lm_head") else None

    def forward(self, input_ids, encoder_hidden_states):
        outputs = self.text_model(
            input_ids=input_ids,
            encoder_hidden_states=encoder_hidden_states,
            use_cache=True,
        )
        hidden_states = outputs.last_hidden_state if hasattr(outputs, "last_hidden_state") else outputs[0]

        if self.lm_head is not None:
            logits = self.lm_head(hidden_states)
        else:
            logits = hidden_states

        past_kv = outputs.past_key_values if hasattr(outputs, "past_key_values") else outputs[1]

        flat_kv = []
        for layer_kv in past_kv:
            for t in layer_kv:
                flat_kv.append(t)

        return (logits,) + tuple(flat_kv)


class DecoderStepWrapper(nn.Module):
    """Wraps the text decoder for subsequent steps (with past_key_values).
    Input:  input_ids [batch, 1], past_key_values (flattened tensors)
    Output: logits [batch, 1, vocab_size], present_key_values (flattened)
    """

    def __init__(self, model, num_layers):
        super().__init__()
        self.text_model = model.thinker.text_model
        self.lm_head = model.lm_head if hasattr(model, "lm_head") else None
        self.num_layers = num_layers

    def forward(self, input_ids, *past_kv_flat):
        past_kv = []
        for i in range(self.num_layers):
            k = past_kv_flat[i * 2]
            v = past_kv_flat[i * 2 + 1]
            past_kv.append((k, v))

        outputs = self.text_model(
            input_ids=input_ids,
            past_key_values=past_kv,
            use_cache=True,
        )
        hidden_states = outputs.last_hidden_state if hasattr(outputs, "last_hidden_state") else outputs[0]

        if self.lm_head is not None:
            logits = self.lm_head(hidden_states)
        else:
            logits = hidden_states

        present_kv = outputs.past_key_values if hasattr(outputs, "past_key_values") else outputs[1]

        flat_kv = []
        for layer_kv in present_kv:
            for t in layer_kv:
                flat_kv.append(t)

        return (logits,) + tuple(flat_kv)


def export_encoder(model, output_dir: str):
    print("Exporting encoder...")
    wrapper = AudioEncoderWrapper(model)
    wrapper.eval()

    mel_features = torch.randn(1, 128, 300)

    out_path = os.path.join(output_dir, "encoder.onnx")
    torch.onnx.export(
        wrapper,
        (mel_features,),
        out_path,
        input_names=["mel_features"],
        output_names=["encoder_hidden_states"],
        dynamic_axes={
            "mel_features": {0: "batch", 2: "mel_length"},
            "encoder_hidden_states": {0: "batch", 1: "seq_length"},
        },
        opset_version=17,
        do_constant_folding=True,
    )
    print(f"  Saved: {out_path} ({os.path.getsize(out_path) / 1e6:.1f} MB)")


def export_decoder_init(model, output_dir: str):
    print("Exporting decoder_init (first step, no KV cache)...")

    wrapper = DecoderInitWrapper(model)
    wrapper.eval()

    batch = 1
    seq_len = 10
    enc_len = 50
    hidden_size = model.config.thinker_config.text_config.hidden_size

    input_ids = torch.randint(0, 1000, (batch, seq_len))
    encoder_hidden_states = torch.randn(batch, enc_len, hidden_size)

    with torch.no_grad():
        sample_outputs = wrapper(input_ids, encoder_hidden_states)

    num_kv = len(sample_outputs) - 1
    output_names = ["logits"]
    dynamic_axes = {
        "input_ids": {0: "batch", 1: "seq_length"},
        "encoder_hidden_states": {0: "batch", 1: "enc_length"},
        "logits": {0: "batch", 1: "seq_length"},
    }

    for i in range(num_kv):
        name = f"present_kv_{i}"
        output_names.append(name)
        dynamic_axes[name] = {0: "batch", 2: "kv_length"}

    out_path = os.path.join(output_dir, "decoder_init.onnx")
    torch.onnx.export(
        wrapper,
        (input_ids, encoder_hidden_states),
        out_path,
        input_names=["input_ids", "encoder_hidden_states"],
        output_names=output_names,
        dynamic_axes=dynamic_axes,
        opset_version=17,
        do_constant_folding=True,
    )
    print(f"  Saved: {out_path} ({os.path.getsize(out_path) / 1e6:.1f} MB)")
    print(f"  KV tensors: {num_kv}")


def export_decoder(model, output_dir: str):
    print("Exporting decoder (subsequent steps, with KV cache)...")

    text_config = model.config.thinker_config.text_config
    num_layers = text_config.num_hidden_layers
    num_kv_heads = text_config.num_key_value_heads
    head_dim = text_config.head_dim

    wrapper = DecoderStepWrapper(model, num_layers)
    wrapper.eval()

    batch = 1
    past_len = 20

    input_ids = torch.randint(0, 1000, (batch, 1))
    past_kv_flat = []
    for _ in range(num_layers):
        past_kv_flat.append(torch.randn(batch, num_kv_heads, past_len, head_dim))  # key
        past_kv_flat.append(torch.randn(batch, num_kv_heads, past_len, head_dim))  # value

    input_names = ["input_ids"]
    dynamic_axes = {
        "input_ids": {0: "batch"},
        "logits": {0: "batch"},
    }
    for i in range(num_layers * 2):
        name = f"past_kv_{i}"
        input_names.append(name)
        dynamic_axes[name] = {0: "batch", 2: "past_length"}

    with torch.no_grad():
        sample_outputs = wrapper(input_ids, *past_kv_flat)

    num_present = len(sample_outputs) - 1
    output_names = ["logits"]
    for i in range(num_present):
        name = f"present_kv_{i}"
        output_names.append(name)
        dynamic_axes[name] = {0: "batch", 2: "kv_length"}

    out_path = os.path.join(output_dir, "decoder.onnx")
    torch.onnx.export(
        wrapper,
        (input_ids, *past_kv_flat),
        out_path,
        input_names=input_names,
        output_names=output_names,
        dynamic_axes=dynamic_axes,
        opset_version=17,
        do_constant_folding=True,
    )
    print(f"  Saved: {out_path} ({os.path.getsize(out_path) / 1e6:.1f} MB)")
    print(f"  Layers: {num_layers}, KV heads: {num_kv_heads}, head_dim: {head_dim}")


def main():
    parser = argparse.ArgumentParser(description="Export Qwen3-ASR to ONNX")
    parser.add_argument("--model-dir", required=True, help="Path to Qwen3-ASR-0.6B model directory")
    parser.add_argument("--output-dir", required=True, help="Directory to save ONNX files")
    args = parser.parse_args()

    os.makedirs(args.output_dir, exist_ok=True)

    model, processor = get_model_and_processor(args.model_dir)

    export_encoder(model, args.output_dir)
    export_decoder_init(model, args.output_dir)
    export_decoder(model, args.output_dir)

    # Save model config info for Go-side reference
    import json
    config_info = {
        "num_layers": model.config.thinker_config.text_config.num_hidden_layers,
        "num_kv_heads": model.config.thinker_config.text_config.num_key_value_heads,
        "head_dim": model.config.thinker_config.text_config.head_dim,
        "hidden_size": model.config.thinker_config.text_config.hidden_size,
        "vocab_size": model.config.thinker_config.text_config.vocab_size,
        "num_attention_heads": model.config.thinker_config.text_config.num_attention_heads,
    }
    info_path = os.path.join(args.output_dir, "onnx_config.json")
    with open(info_path, "w") as f:
        json.dump(config_info, f, indent=2)
    print(f"\nONNX config info saved: {info_path}")
    print("Done! All ONNX files exported successfully.")


if __name__ == "__main__":
    main()
