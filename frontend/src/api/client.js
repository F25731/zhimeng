import axios from "axios";

const client = axios.create({
  baseURL: "/api",
  timeout: 15000,
  withCredentials: true
});

client.interceptors.request.use((config) => {
  const csrfToken = sessionStorage.getItem("control_csrf_token");
  if (csrfToken) {
    config.headers["X-CSRF-Token"] = csrfToken;
  }
  return config;
});

client.interceptors.response.use(
  (response) => response,
  (error) => {
    const payload = error.response && error.response.data;
    if (payload && payload.message) {
      error.userMessage = payload.message;
    }
    return Promise.reject(error);
  }
);

export default client;
