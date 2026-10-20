import { allows, type Session, type WorkspacePage } from "../api";

// pageTitle supplies the consistent label used in workspace headings and breadcrumbs.
export function pageTitle(page: WorkspacePage): string {
  return {
    overview: "Overview",
    cluster: "Cluster",
    buckets: "Buckets",
    keys: "Access keys",
    tokens: "Admin tokens",
    profile: "My profile",
  }[page];
}

// canVisitPage checks which workspace destinations this Garage credential can use.
export function canVisitPage(session: Session, page: WorkspacePage): boolean {
  switch (page) {
    case "overview":
      return session.mode === "admin";
    case "cluster":
      return allows(session, "GetClusterStatus");
    case "keys":
      return allows(session, "ListKeys");
    case "tokens":
      return allows(session, "ListAdminTokens");
    default:
      return true;
  }
}
