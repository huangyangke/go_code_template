import type { Route } from "./+types/home";

export function meta({}: Route.MetaArgs) {
  return [{ title: "App" }, { name: "description", content: "Welcome" }];
}

export default function Home() {
  return (
    <main className="flex min-h-screen items-center justify-center">
      <h1 className="text-2xl font-bold">Home</h1>
    </main>
  );
}
