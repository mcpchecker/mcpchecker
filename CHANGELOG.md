# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added
- Token usage comparison in diff command (total tokens and MCP schema tokens)

### Changed

### Fixed

## [0.0.5]

### Added
- ACP (Agent Control Protocol) client library (#113)
- ACP runner for agents (#134)
- ACP support for builtin OpenAI agent (#164)
- Template substitution in llmJudge steps (#170)
- IDs for all task steps (#171)
- Environment variable support for MCP configuration (#131)
- 'as' alias support for task definitions (#140)

### Changed
- Timeout configuration on extension call steps (#169)
- Refactored MCP client management to dedicated package for more reliable connections and lifecycle handling (#144)

### Fixed
- Mutex copy issue in protocol.Operation (#143)
- Release workflows handle backticks in changelog (#137)

## [0.0.4]

### Added
- Verify command to validate results meet pass rate thresholds (#122)
- Summary command with text, JSON, and GitHub Actions output formats (#122)
- Diff command to compare two evaluation runs with regression detection (#122)
- Labels support for task definitions with label-selector flag for filtering (#115)

### Changed
- Command `eval` is now renamed to `check` in the CLI (#121)
- Extracted mcpchecker setup into reusable GitHub Action (#128)

### Fixed
- GitHub Action uses correct `check` command (#123)
- Functional tests pass after labels and multi-task changes (#129)

### Documentation
- Added sections on why MCPChecker and links to downloads and quickstarts (#120)

## [0.0.3]

### Changed
- Renamed project from gevals to mcpchecker, migrated to mcpchecker github org

## [0.0.2]

### Added
- Extension support with Go extension SDK (#79)
- Gemini agent support (#69)
- Builtin steps for task execution (#56)
- View command for eval results (#36)
- Functional test framework (#71)
- Dependabot for automated dependency updates (#64)

### Changed
- Updated modelcontextprotocol/go-sdk to v1.2.0 (#75)
- Updated Go version to 1.25.x in GitHub Action (#80)
- Bumped actions/checkout from v5 to v6 (#66, #74)
- Bumped actions/upload-artifact from v5 to v6 (#68)

### Fixed
- Action correctly picks up pinned version when set (#81)
- Race conditions in mcpproxy (#70)

## [0.0.1]

### Added
- Initial release of gevals
- MCP server evaluation framework
- Support for multiple agent types (Claude Code, OpenAI)
- Kubernetes MCP server examples
- LLM judge for evaluating responses
- Release workflows for automated publishing
- GitHub Action for running gevals evaluations
- Support for nightly releases
