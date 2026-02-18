# Azure Blob Storage Implementation Plan

## Overview
Implement Azure Blob Storage support for Harness CI cache operations with OIDC token-based authentication provided by Harness CI backend.

## Architecture Analysis
Based on existing S3/GCS implementations:
- Follow storage interface pattern in `storage/backend/`
- Leverage Azure SDK for Go (azure-storage-blob, azure-identity)
- Integrate with existing cache operations (rebuild, restore, flush)
- Maintain compatibility with archive formats (tar, gzip, zstd)

## Current State Analysis

### Existing Azure Implementation
The Azure backend already exists with:
- Basic Azure Blob Storage operations (Get, Put, Exists)
- Shared key authentication (AccountName + AccountKey)
- Uses legacy `azure-storage-blob-go` SDK
- Missing OIDC token support

### Required Changes
1. **Configuration Updates**: Add OIDC token fields to Azure config
2. **Authentication Enhancement**: Add OIDC token authentication with fallback
3. **SDK Migration**: Update to modern Azure SDK for better OIDC support
4. **CLI Integration**: Add new environment variables and flags

## Implementation Strategy

### 1. Azure Storage Backend Structure (Existing)
```
storage/backend/azure/
├── azure.go          # Main Azure storage implementation ✅ EXISTS
├── config.go         # Azure-specific configuration ✅ EXISTS  
└── azure_test.go     # Unit tests ✅ EXISTS
```

### 2. Authentication Flow
**Primary (OIDC Token):**
- Use PLUGIN_AZURE_OIDC_TOKEN provided by Harness CI
- Authenticate with Azure Identity SDK using token credentials
- No token refresh logic (Harness CI manages lifecycle)

**Fallback (Manual Credentials):**
- Use CLIENT_ID, CLIENT_SECRET, TENANT_ID for service principal auth
- Handle cases where OIDC token is not available

### 3. Configuration Integration
- Add Azure backend to main CLI configuration
- Support environment variable mapping
- Integrate with existing backend selection logic

### 4. Cache Operations Implementation
- **Save (Rebuild):** Upload archives to Azure Blob Storage
- **Restore:** Download and extract archives from Azure Blob Storage  
- **Flush:** Delete specific cache entries from Azure containers

### 5. Error Handling Scenarios
- Authentication failures (invalid token, expired credentials)
- Storage account/container not found
- Insufficient permissions
- Network connectivity issues
- Blob size limits

## Implementation Tasks

### Phase 1: Core Azure Backend
1. Analyze existing S3/GCS backend implementations
2. Create Azure storage backend structure
3. Implement authentication with OIDC + fallback
4. Add basic blob upload/download operations

### Phase 2: Integration
5. Add Azure backend to main configuration
6. Integrate with existing cache operations
7. Add CLI flags and environment variable support
8. Implement error handling and logging

### Phase 3: Testing & Polish
9. Create comprehensive unit tests
10. Add integration tests with real Azure storage
11. Performance testing and optimization
12. Documentation updates

## Success Criteria
- ✅ Authentication works with pre-fetched OIDC tokens
- ✅ No OIDC generation/refresh logic in plugin
- ✅ Cache save/restore operations work with Azure Blob Storage
- ✅ Fallback to manual credentials functions correctly
- ✅ Performance within 20% of S3/GCS implementations
- ✅ Clear error messages for Azure-specific failures
- ✅ Zero breaking changes to existing functionality

## Dependencies
- github.com/Azure/azure-storage-blob-go
- github.com/Azure/azure-sdk-for-go/sdk/azidentity
- Compatible with existing Go modules and vendoring

## Key Design Decisions
1. **Token Trust:** Plugin trusts OIDC tokens from Harness CI (no validation)
2. **Container Auto-creation:** Support automatic container creation if missing
3. **Blob Naming:** Follow pattern `{prefix}/{cache_key}/{filename}`
4. **Archive Compatibility:** Maintain full compatibility with existing formats
5. **Error Propagation:** Return meaningful Azure-specific error messages

## Implementation Notes
- Follow existing backend patterns for consistency
- Leverage Azure SDK best practices
- Implement retry logic with exponential backoff
- Support Azure Storage regions and tiers appropriately
- Log authentication method used (OIDC vs manual)

## Next Steps
1. Review existing S3/GCS implementations for patterns
2. Begin Azure backend core implementation
3. Iterate on authentication and storage operations
4. Add comprehensive testing coverage