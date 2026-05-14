## T1 Blockers

### RESOLVED: Real ONNX Implementation Complete

**Status**: RESOLVED ✓

**What Was Done**:
- Implemented real ONNX backend using onnxruntime_go v1.16.0 + tokenizers v1.27.0
- All native dependencies confirmed installed:
  - libonnxruntime.so.1.20.1 at /usr/local/lib/
  - libtokenizers.a at /usr/local/lib/
  - Model files at /opt/iris-models/paraphrase-MiniLM-L3-v2/
- All tests PASS (11 unit + 3 integration)
- Thread-safe inference with mutex protection
- Graceful resource cleanup

**Version Compatibility**:
- ORT 1.20.1 requires API version 20
- onnxruntime_go v1.16.0 is compatible (v1.17+ requires API v22)
- Downgraded from v1.30.1 to v1.16.0 for compatibility

**Production Ready**:
- No fallback logic needed
- Real embeddings available immediately
- T2-T5 can proceed with full implementation

