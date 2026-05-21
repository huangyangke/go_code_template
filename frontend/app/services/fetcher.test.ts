import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { ApiCode } from "~/types/api";
import apiClient from "./api";
import { swrFetcher } from "./fetcher";

vi.mock("./api");

describe("swrFetcher", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("returns data when code is SUCCESS", async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({
      code: ApiCode.SUCCESS,
      data: { id: 1 },
      msg: "",
      task_id: "",
    } as never);

    const result = await swrFetcher("/test");
    expect(result).toEqual({ id: 1 });
  });

  it("throws when code is not SUCCESS", async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({
      code: ApiCode.UNAUTHORIZED,
      data: null,
      msg: "未授权",
      task_id: "",
    } as never);

    await expect(swrFetcher("/test")).rejects.toThrow("未授权");
  });

  it("throws generic message when msg is empty", async () => {
    vi.mocked(apiClient.get).mockResolvedValueOnce({
      code: ApiCode.INTERNAL_ERROR,
      data: null,
      msg: "",
      task_id: "",
    } as never);

    await expect(swrFetcher("/test")).rejects.toThrow("请求失败");
  });
});
