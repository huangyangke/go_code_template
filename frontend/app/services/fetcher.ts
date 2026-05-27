import type { SWRConfiguration } from "swr";
import { ApiCode, type ApiResponse } from "~/types/api";
import apiClient from "./api";

export async function swrFetcher<T>(url: string): Promise<T> {
  const res = await apiClient.get<ApiResponse<T>>(url);
  const body = res as unknown as ApiResponse<T>;
  if (body.code !== ApiCode.SUCCESS) {
    throw new Error(body.msg || "请求失败");
  }
  // backend normalizes nil data to [] for list endpoints, but some
  // operations (delete, update) legitimately return null data.
  // Cast is safe because the caller controls T via useSWR<T>.
  return body.data as T;
}

export const swrConfig: SWRConfiguration = {
  fetcher: swrFetcher,
  revalidateOnFocus: false,
  shouldRetryOnError: false,
};
