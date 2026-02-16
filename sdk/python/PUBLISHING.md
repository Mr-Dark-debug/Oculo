# Publishing the Oculo SDK to PyPI

This guide describes how to build and publish the Oculo Python SDK to PyPI.

## Prerequisites

Ensure you have the necessary build tools installed:

```bash
pip install build twine
```

## 1. Build the Package

Navigate to the `sdk/python` directory and run the build command:

```bash
cd sdk/python
python -m build
```

This will create a `dist/` directory containing two files:
- `oculo_sdk-0.1.0-py3-none-any.whl` (Wheel)
- `oculo_sdk-0.1.0.tar.gz` (Source Archive)

## 2. Test with TestPyPI (Recommended)

Before publishing to the real PyPI, it's good practice to upload to TestPyPI.

1. Create an account at [TestPyPI](https://test.pypi.org/account/register/).
2. Create an API token [here](https://test.pypi.org/manage/account/).
3. Upload the package:

```bash
python -m twine upload --repository testpypi dist/*
```

## 3. Publish to PyPI

When you are ready to release to the public:

1. Create an account at [PyPI](https://pypi.org/account/register/).
2. Create an API token [here](https://pypi.org/manage/account/).
3. Upload the package:

```bash
python -m twine upload dist/*
```

## Versioning

To publish a new version:
1. Update the `version` field in `pyproject.toml`.
2. Update `__version__` in `oculo/__init__.py`.
3. Repeat the build and upload steps.
