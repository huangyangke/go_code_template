import axios from "axios";
import { API_BASE_URL } from "~/lib/constants";

const apiClient = axios.create({
  baseURL: API_BASE_URL,
  timeout: 10_000,
  headers: { "Content-Type": "application/json" },
});

apiClient.interceptors.response.use(
  (response) => response.data,
  (error) => {
    const data = error?.response?.data;
    return Promise.reject(data ?? error);
  },
);

export default apiClient;
