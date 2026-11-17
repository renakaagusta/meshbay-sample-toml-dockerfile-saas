// Where the api lives.
//
// API_URL wins when it is set (a project variable, or the manifest default).
// Otherwise the address is derived from MESHBAY_PROJECT, which Meshbay injects
// into every service of a manifest deploy: a service on the private network is
// `<project>-<service>.internal`, so the api is `<project>-api.internal:8080`.
export function resolveApiUrl(environment = process.env) {
  const configured = (environment.API_URL || "").trim();
  if (configured !== "") {
    return configured.replace(/\/+$/, "");
  }
  const project = (environment.MESHBAY_PROJECT || "").trim();
  if (project !== "") {
    return `http://${project}-api.internal:8080`;
  }
  return "http://localhost:8080";
}
