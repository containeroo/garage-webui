package admin

import "net/http"

// Operation identifies one supported Garage Admin API endpoint.
// A closed set prevents a caller from choosing arbitrary upstream paths or methods.
type Operation string

const (
	OpConnectClusterNodes         Operation = "ConnectClusterNodes"
	OpUpdateClusterLayout         Operation = "UpdateClusterLayout"
	OpPreviewClusterLayoutChanges Operation = "PreviewClusterLayoutChanges"
	OpApplyClusterLayout          Operation = "ApplyClusterLayout"
	OpRevertClusterLayout         Operation = "RevertClusterLayout"
	OpGetClusterHealth            Operation = "GetClusterHealth"
	OpGetClusterStatus            Operation = "GetClusterStatus"
	OpGetClusterLayout            Operation = "GetClusterLayout"
	OpGetClusterStatistics        Operation = "GetClusterStatistics"
	OpGetCurrentAdminTokenInfo    Operation = "GetCurrentAdminTokenInfo"
	OpListAdminTokens             Operation = "ListAdminTokens"
	OpGetAdminTokenInfo           Operation = "GetAdminTokenInfo"
	OpCreateAdminToken            Operation = "CreateAdminToken"
	OpUpdateAdminToken            Operation = "UpdateAdminToken"
	OpDeleteAdminToken            Operation = "DeleteAdminToken"
	OpListBuckets                 Operation = "ListBuckets"
	OpGetBucketInfo               Operation = "GetBucketInfo"
	OpListKeys                    Operation = "ListKeys"
	OpGetKeyInfo                  Operation = "GetKeyInfo"
	OpCreateBucket                Operation = "CreateBucket"
	OpDeleteBucket                Operation = "DeleteBucket"
	OpCreateKey                   Operation = "CreateKey"
	OpDeleteKey                   Operation = "DeleteKey"
	OpUpdateKey                   Operation = "UpdateKey"
	OpAllowBucketKey              Operation = "AllowBucketKey"
	OpDenyBucketKey               Operation = "DenyBucketKey"
	OpUpdateBucket                Operation = "UpdateBucket"
	OpAddBucketAlias              Operation = "AddBucketAlias"
	OpRemoveBucketAlias           Operation = "RemoveBucketAlias"
)

// Method returns the fixed HTTP method for a permitted Garage operation.
func (op Operation) Method() (string, bool) {
	switch op {
	case OpConnectClusterNodes,
		OpUpdateClusterLayout,
		OpPreviewClusterLayoutChanges,
		OpApplyClusterLayout,
		OpRevertClusterLayout,
		OpCreateAdminToken,
		OpUpdateAdminToken,
		OpDeleteAdminToken,
		OpCreateBucket,
		OpDeleteBucket,
		OpCreateKey,
		OpDeleteKey,
		OpUpdateKey,
		OpAllowBucketKey,
		OpDenyBucketKey,
		OpUpdateBucket,
		OpAddBucketAlias,
		OpRemoveBucketAlias:
		return http.MethodPost, true
	case OpGetClusterHealth,
		OpGetClusterStatus,
		OpGetClusterLayout,
		OpGetClusterStatistics,
		OpGetCurrentAdminTokenInfo,
		OpListAdminTokens,
		OpGetAdminTokenInfo,
		OpListBuckets,
		OpGetBucketInfo,
		OpListKeys,
		OpGetKeyInfo:
		return http.MethodGet, true
	default:
		return "", false
	}
}

// ParseOperation accepts only known Admin API operations.
func ParseOperation(raw string) (Operation, bool) {
	op := Operation(raw)
	_, ok := op.Method()

	return op, ok
}
