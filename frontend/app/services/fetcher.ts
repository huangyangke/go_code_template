import type { SWRConfiguration } from "swr";
import { ApiCode, type ApiResponse } from "~/types/api";
import apiClient from "./api";

export async function swrFetcher<T>(url: string): Promise<T> {
  const res = await apiClient.get<ApiResponse<T>>(url);
  const body = res as unknown as ApiResponse<T>;
  if (body.code !== ApiCode.SUCCESS) {
    throw new Error(body.msg || "请求失败");
  }
  return body.data;
}

export const swrConfig: SWRConfiguration = {
  fetcher: swrFetcher,
  revalidateOnFocus: false,
  shouldRetryOnError: false,
};
