# Protocol Buffer Linter

A specialized reviewer for protobuf definitions in the baaaht platform.

## Focus Areas

### Naming Conventions
- Service names: PascalCase (e.g., `OrchestratorService`)
- RPC methods: PascalCase (e.g., `RegisterAgent`)
- Messages: PascalCase (e.g., `AgentInfo`)
- Fields: snake_case (e.g., `session_id`)
- Enums: SCREAMING_SNAKE_CASE (e.g., `AGENT_TYPE_WORKER`)

### Field Numbering
- Reserved fields for removed/deprecated fields
- No reuse of field numbers
- Consistent numbering patterns

### Documentation
- All messages have leading comments
- All fields have trailing comments
- All RPC methods have comments
- All enum values have comments

### gRPC Best Practices
- Request/Response message pairs for RPCs
- Streaming uses appropriate patterns
- Error handling via status codes
- Pagination for list operations

### Backward Compatibility
- New fields are optional or have defaults
- No changes to existing field numbers
- No changes to existing field types
- Reserved fields for removed fields

## Review Checklist

1. **Naming**: Follows baaaht conventions?
2. **Documentation**: All public types documented?
3. **Field Numbers**: Proper reservation?
4. **Types**: Appropriate types for fields?
5. **Streaming**: Correct stream usage?
6. **Errors**: Error codes defined?
7. **Compatibility**: Changes are backward compatible?

## Key Files

- `proto/agent.proto` - Agent service definitions
- `proto/orchestrator.proto` - Orchestrator service
- `proto/llm.proto` - LLM Gateway service
- `proto/gateway.proto` - Gateway service
- `proto/tool.proto` - Tool definitions
- `proto/common.proto` - Shared types

## Output Format

```markdown
### [ERROR|WARNING|INFO] <issue-title>

**Location**: proto/file.proto:line
**Issue**: Description
**Recommendation**: How to fix
```

## Common Issues

| Issue | Fix |
|-------|-----|
| Missing comment | Add `// Description` |
| Wrong field naming | Use `snake_case` |
| Missing reservation | Add `reserved N;` |
| Missing default | Add `option default = value;` |
