// Package garage defines Garage operation and access policies.
package garage

// operations lists the Garage endpoints the browser may access.
var operations = map[string]string{
	"ConnectClusterNodes":         "POST",
	"UpdateClusterLayout":         "POST",
	"PreviewClusterLayoutChanges": "POST",
	"ApplyClusterLayout":          "POST",
	"RevertClusterLayout":         "POST",
	"GetClusterHealth":            "GET",
	"GetClusterStatus":            "GET",
	"GetClusterLayout":            "GET",
	"GetClusterStatistics":        "GET",
	"ListBuckets":                 "GET",
	"GetBucketInfo":               "GET",
	"ListKeys":                    "GET",
	"GetKeyInfo":                  "GET",
	"CreateBucket":                "POST",
	"DeleteBucket":                "POST",
	"CreateKey":                   "POST",
	"DeleteKey":                   "POST",
	"UpdateKey":                   "POST",
	"AllowBucketKey":              "POST",
	"DenyBucketKey":               "POST",
	"UpdateBucket":                "POST",
	"AddBucketAlias":              "POST",
	"RemoveBucketAlias":           "POST",
}

// OperationMethod returns the permitted method for an Admin API operation.
func OperationMethod(operation string) (method string, allowed bool) {
	method, ok := operations[operation]
	return method, ok
}

// requiresClusterAdmin identifies cluster operations reserved for administrators.
func requiresClusterAdmin(operation string) bool {
	switch operation {
	case "ConnectClusterNodes",
		"UpdateClusterLayout",
		"PreviewClusterLayoutChanges",
		"ApplyClusterLayout",
		"RevertClusterLayout":
		return true
	default:
		return false
	}
}

// CanAccess reports whether a role may use the requested operation method.
func CanAccess(role, method string) bool {
	return CanModify(role) || (role == "viewer" && method == "GET")
}

// CanModify reports whether a role can change Garage resources.
func CanModify(role string) bool {
	return role == "admin" || role == "editor"
}
