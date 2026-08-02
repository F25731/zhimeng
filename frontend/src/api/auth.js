import client from "./client";

export async function login(payload) {
  const response = await client.post("/admin/auth/login", payload);
  return response.data.data;
}

export async function me() {
  const response = await client.get("/admin/auth/me");
  return response.data.data;
}

export async function logout() {
  const response = await client.post("/admin/auth/logout");
  return response.data.data;
}
