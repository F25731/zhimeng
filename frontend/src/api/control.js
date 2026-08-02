import client from "./client";

const data = (response) => response.data.data;

export const verifyCode = (code) => client.post("/public/provision/codes/verify", { code }).then(data);
export const checkDomain = (prefix, token) => client.post("/public/provision/domain/check", { prefix }, { headers: { "X-Provision-Token": token } }).then(data);
export const uploadLogo = (file, token) => {
  const form = new FormData();
  form.append("file", file);
  return client.post("/public/provision/logo/upload", form, { headers: { "X-Provision-Token": token } }).then(data);
};
export const importLogo = (url, token) => client.post("/public/provision/logo/import", { url }, { headers: { "X-Provision-Token": token } }).then(data);
export const createProvisionJob = (payload, token) => client.post("/public/provision/jobs", payload, { headers: { "X-Provision-Token": token } }).then(data);
export const getJob = (id, token) => client.get(`/public/provision/jobs/${id}`, { headers: { "X-Provision-Token": token } }).then(data);
export const retryPublicJob = (id, token) => client.post(`/public/provision/jobs/${id}/retry`, null, { headers: { "X-Provision-Token": token } }).then(data);

export const dashboard = () => client.get("/admin/dashboard").then(data);
export const listCodes = (params) => client.get("/admin/codes", { params }).then(data);
export const createCodes = (payload) => client.post("/admin/codes", payload).then(data);
export const revokeCode = (id) => client.post(`/admin/codes/${id}/revoke`).then(data);
export const deleteCode = (id) => client.delete(`/admin/codes/${id}`).then(data);
export const listSites = (params) => client.get("/admin/sites", { params }).then(data);
export const getSite = (id) => client.get(`/admin/sites/${id}`).then(data);
export const getSiteMetrics = (id) => client.get(`/admin/sites/${id}/metrics`).then(data);
export const getSiteChannels = (id) => client.get(`/admin/sites/${id}/channels`).then(data);
export const siteAction = (id, action, version, confirmation) => client.post(`/admin/sites/${id}/${action}`, { version, confirmation }).then(data);
export const listJobs = (params) => client.get("/admin/jobs", { params }).then(data);
export const getAdminJob = (id) => client.get(`/admin/jobs/${id}`).then(data);
export const retryJob = (id) => client.post(`/admin/jobs/${id}/retry`).then(data);
export const listVersions = () => client.get("/admin/versions").then(data);
export const createVersion = (payload) => client.post("/admin/versions", payload).then(data);
export const publishVersion = (id) => client.post(`/admin/versions/${id}/publish`).then(data);
export const listNodes = () => client.get("/admin/nodes").then(data);
export const listAudit = (params) => client.get("/admin/audit", { params }).then(data);
