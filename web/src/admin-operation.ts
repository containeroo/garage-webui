/** Garage Admin API operations exposed by the console backend. */
export const AdminOperation = {
  /** Authorizes the Garage ConnectClusterNodes operation. */
  ConnectClusterNodes: "ConnectClusterNodes",
  /** Authorizes the Garage UpdateClusterLayout operation. */
  UpdateClusterLayout: "UpdateClusterLayout",
  /** Authorizes the Garage PreviewClusterLayoutChanges operation. */
  PreviewClusterLayoutChanges: "PreviewClusterLayoutChanges",
  /** Authorizes the Garage ApplyClusterLayout operation. */
  ApplyClusterLayout: "ApplyClusterLayout",
  /** Authorizes the Garage RevertClusterLayout operation. */
  RevertClusterLayout: "RevertClusterLayout",
  /** Authorizes the Garage GetClusterHealth operation. */
  GetClusterHealth: "GetClusterHealth",
  /** Authorizes the Garage GetClusterStatus operation. */
  GetClusterStatus: "GetClusterStatus",
  /** Authorizes the Garage GetClusterLayout operation. */
  GetClusterLayout: "GetClusterLayout",
  /** Authorizes the Garage GetClusterStatistics operation. */
  GetClusterStatistics: "GetClusterStatistics",
  /** Authorizes the Garage GetCurrentAdminTokenInfo operation. */
  GetCurrentAdminTokenInfo: "GetCurrentAdminTokenInfo",
  /** Authorizes the Garage ListAdminTokens operation. */
  ListAdminTokens: "ListAdminTokens",
  /** Authorizes the Garage GetAdminTokenInfo operation. */
  GetAdminTokenInfo: "GetAdminTokenInfo",
  /** Authorizes the Garage CreateAdminToken operation. */
  CreateAdminToken: "CreateAdminToken",
  /** Authorizes the Garage UpdateAdminToken operation. */
  UpdateAdminToken: "UpdateAdminToken",
  /** Authorizes the Garage DeleteAdminToken operation. */
  DeleteAdminToken: "DeleteAdminToken",
  /** Authorizes the Garage ListBuckets operation. */
  ListBuckets: "ListBuckets",
  /** Authorizes the Garage GetBucketInfo operation. */
  GetBucketInfo: "GetBucketInfo",
  /** Authorizes the Garage ListKeys operation. */
  ListKeys: "ListKeys",
  /** Authorizes the Garage GetKeyInfo operation. */
  GetKeyInfo: "GetKeyInfo",
  /** Authorizes the Garage CreateBucket operation. */
  CreateBucket: "CreateBucket",
  /** Authorizes the Garage DeleteBucket operation. */
  DeleteBucket: "DeleteBucket",
  /** Authorizes the Garage CreateKey operation. */
  CreateKey: "CreateKey",
  /** Authorizes the Garage DeleteKey operation. */
  DeleteKey: "DeleteKey",
  /** Authorizes the Garage UpdateKey operation. */
  UpdateKey: "UpdateKey",
  /** Authorizes the Garage AllowBucketKey operation. */
  AllowBucketKey: "AllowBucketKey",
  /** Authorizes the Garage DenyBucketKey operation. */
  DenyBucketKey: "DenyBucketKey",
  /** Authorizes the Garage UpdateBucket operation. */
  UpdateBucket: "UpdateBucket",
  /** Authorizes the Garage AddBucketAlias operation. */
  AddBucketAlias: "AddBucketAlias",
  /** Authorizes the Garage RemoveBucketAlias operation. */
  RemoveBucketAlias: "RemoveBucketAlias",
} as const;

/** An operation accepted by the backend Garage proxy. */
export type AdminOperation =
  (typeof AdminOperation)[keyof typeof AdminOperation];
