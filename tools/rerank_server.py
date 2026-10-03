#!/usr/bin/env python3
"""Local cross-encoder reranker (bge-reranker-v2-m3, ONNX, CPU) over HTTP.

POST /score  {"pairs": [[query, passage], ...]}  ->  {"scores": [p, ...]}
Scores are sigmoid(logit) in [0, 1], the model's standard relevance score.

Model files (not tracked; ~2.3 GB) come from
huggingface.co/onnx-community/bge-reranker-v2-m3-ONNX (onnx/model.onnx,
onnx/model.onnx_data, tokenizer.json) in --model-dir.

    python3 tools/rerank_server.py --model-dir .cache/rerank --port 8765
"""
import argparse
import json
import math
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from threading import Lock

import numpy as np
import onnxruntime as ort
from tokenizers import Tokenizer

MAX_LEN = 512


def load(model_dir, threads, model_file="model.onnx"):
    tok = Tokenizer.from_file(f"{model_dir}/tokenizer.json")
    tok.enable_truncation(max_length=MAX_LEN, strategy="only_second")
    so = ort.SessionOptions()
    so.intra_op_num_threads = threads
    sess = ort.InferenceSession(f"{model_dir}/{model_file}", so, providers=["CPUExecutionProvider"])
    return tok, sess


def score(tok, sess, pairs, batch=16):
    """Scores pairs in order. Pairs are batched by similar length so little
    time is spent on padding."""
    inputs = {i.name for i in sess.get_inputs()}
    enc_all = tok.encode_batch([(q, p) for q, p in pairs])
    order = sorted(range(len(pairs)), key=lambda i: len(enc_all[i].ids))
    out = [0.0] * len(pairs)
    for s in range(0, len(order), batch):
        idx = order[s:s + batch]
        enc = [enc_all[i] for i in idx]
        n = max(len(e.ids) for e in enc)
        ids = np.ones((len(enc), n), dtype=np.int64)  # XLM-R pad id = 1
        mask = np.zeros((len(enc), n), dtype=np.int64)
        for i, e in enumerate(enc):
            ids[i, :len(e.ids)] = e.ids
            mask[i, :len(e.ids)] = 1
        feed = {"input_ids": ids, "attention_mask": mask}
        if "token_type_ids" in inputs:
            feed["token_type_ids"] = np.zeros_like(ids)
        logits = sess.run(None, feed)[0].reshape(-1)
        for i, x in zip(idx, logits):
            out[i] = 1 / (1 + math.exp(-float(x)))
    return out


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--model-dir", default=".cache/rerank")
    ap.add_argument("--port", type=int, default=8765)
    ap.add_argument("--threads", type=int, default=4)
    ap.add_argument("--model-file", default="model.onnx", help="model.onnx (fp32) or model_int8.onnx (int8, faster on CPU)")
    args = ap.parse_args()
    tok, sess = load(args.model_dir, args.threads, args.model_file)
    lock = Lock()  # one inference at a time; ORT already uses all threads

    class H(BaseHTTPRequestHandler):
        def do_POST(self):
            body = json.loads(self.rfile.read(int(self.headers["Content-Length"])))
            with lock:
                scores = score(tok, sess, body["pairs"])
            data = json.dumps({"scores": scores}).encode()
            self.send_response(200)
            self.send_header("Content-Type", "application/json")
            self.send_header("Content-Length", str(len(data)))
            self.end_headers()
            self.wfile.write(data)

        def log_message(self, *a):
            pass

    print(f"reranker ready on :{args.port}", flush=True)
    ThreadingHTTPServer(("127.0.0.1", args.port), H).serve_forever()


if __name__ == "__main__":
    main()
