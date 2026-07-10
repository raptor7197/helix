from setuptools import setup, find_packages

# The `vit` CLI is a Go binary (cmd/vit) — build it with:
#   go build -o ~/.vit/bin/vit ./cmd/vit
# This package only ships the Python modules the DaVinci Resolve plugin
# imports (serializer/deserializer/models + thin shims over the Go binary).
setup(
    name="vit",
    version="0.1.0",
    description="Git for Video Editing — version control timeline metadata, not media files",
    packages=find_packages(),
    python_requires=">=3.8",
    install_requires=[
        "PySide6",
    ],
    extras_require={},
)
