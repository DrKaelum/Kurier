export function App() {
  const stage = import.meta.env.VITE_KURIER_STAGE ?? "local";

  return (
    <main>
      <p className="eyebrow">Agent-ready API evidence</p>
      <h1>Kurier</h1>
      <p>
        A focused workspace for testing, debugging, and safely inspecting API
        executions.
      </p>
      <p className="stage">Environment: {stage}</p>
    </main>
  );
}
