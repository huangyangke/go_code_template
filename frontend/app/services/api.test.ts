import MockAdapter from "axios-mock-adapter";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import apiClient from "./api";

describe("apiClient interceptors", () => {
  let mock: MockAdapter;

  beforeEach(() => {
    mock = new MockAdapter(apiClient);
  });

  afterEach(() => {
    mock.restore();
  });

  it("unwraps response.data on success", async () => {
    const payload = { code: 200, data: { id: 1 }, msg: "", task_id: "" };
    mock.onGet("/test").reply(200, payload);

    const result = await apiClient.get("/test");
    expect(result).toEqual(payload);
  });

  it("rejects with response.data when server returns error body", async () => {
    const errorBody = { code: 10007, msg: "未授权", data: null, task_id: "" };
    mock.onGet("/test").reply(401, errorBody);

    await expect(apiClient.get("/test")).rejects.toEqual(errorBody);
  });

  it("rejects with axios error when no response body", async () => {
    mock.onGet("/test").networkError();

    await expect(apiClient.get("/test")).rejects.toMatchObject({
      message: expect.any(String),
    });
  });
});
