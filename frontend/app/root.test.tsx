import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ErrorBoundary } from "./root";

vi.mock("react-router", async (importOriginal) => {
  const actual = await importOriginal<typeof import("react-router")>();
  return { ...actual, isRouteErrorResponse: vi.fn() };
});

import { isRouteErrorResponse } from "react-router";

const params = {};

describe("ErrorBoundary", () => {
  it("renders 404 message for route error with status 404", () => {
    vi.mocked(isRouteErrorResponse).mockReturnValue(true);
    render(<ErrorBoundary error={{ status: 404, statusText: "" } as never} params={params} />);

    expect(screen.getByRole("heading", { name: "404" })).toBeInTheDocument();
    expect(screen.getByText("The requested page could not be found.")).toBeInTheDocument();
  });

  it("renders Error message for non-404 route error", () => {
    vi.mocked(isRouteErrorResponse).mockReturnValue(true);
    render(
      <ErrorBoundary
        error={{ status: 500, statusText: "Server Error" } as never}
        params={params}
      />,
    );

    expect(screen.getByRole("heading", { name: "Error" })).toBeInTheDocument();
    expect(screen.getByText("Server Error")).toBeInTheDocument();
  });

  it("falls back to default details when statusText is empty", () => {
    vi.mocked(isRouteErrorResponse).mockReturnValue(true);
    render(<ErrorBoundary error={{ status: 500, statusText: "" } as never} params={params} />);

    expect(screen.getByText("An unexpected error occurred.")).toBeInTheDocument();
  });

  it("renders generic Oops for unknown errors", () => {
    vi.mocked(isRouteErrorResponse).mockReturnValue(false);
    render(<ErrorBoundary error={new Error("something broke")} params={params} />);

    expect(screen.getByRole("heading", { name: "Oops!" })).toBeInTheDocument();
    // In DEV mode (vitest), error.message is shown instead of the default text
    expect(screen.getByText("something broke")).toBeInTheDocument();
  });

  it("renders default details for non-Error unknown errors", () => {
    vi.mocked(isRouteErrorResponse).mockReturnValue(false);
    render(<ErrorBoundary error="plain string error" params={params} />);

    expect(screen.getByRole("heading", { name: "Oops!" })).toBeInTheDocument();
    expect(screen.getByText("An unexpected error occurred.")).toBeInTheDocument();
  });
});
