export interface ApiResponse<T = unknown> {
  code: number;
  data: T | null;
  msg: string;
  task_id: string;
}

export const ApiCode = {
  SUCCESS: 200,
  BAD_REQUEST: 10000,
  PARAMETER_ERROR: 10001,
  METHOD_NOT_ALLOWED: 10002,
  NOT_FOUND: 10003,
  RATE_LIMITED: 10004,
  INTERNAL_ERROR: 10005,
  USER_NOT_FOUND: 10006,
  UNAUTHORIZED: 10007,
  FORBIDDEN: 10008,
  CONFLICT: 10009,
} as const;
