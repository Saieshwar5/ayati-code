interface ErrorViewProps {
  message: string;
}

export function LoadingView() {
  return (
    <main>
      <section className="center-card">
        <p className="eyebrow">Local coding workspace</p>
        <h1>Preparing Ayati…</h1>
      </section>
    </main>
  );
}

export function ErrorView({ message }: ErrorViewProps) {
  return (
    <main>
      <section className="center-card" role="alert">
        <p className="eyebrow">Unable to start</p>
        <h1>{message}</h1>
      </section>
    </main>
  );
}

export function ConfigureView() {
  return (
    <main>
      <section className="center-card">
        <p className="eyebrow">GitHub App required</p>
        <h1>Connect Ayati to GitHub</h1>
        <p className="muted">
          Start Ayati with <code>AYATI_GITHUB_CLIENT_ID</code> and{" "}
          <code>AYATI_GITHUB_CLIENT_SECRET</code>, using{" "}
          <code>http://127.0.0.1:8080/auth/github/callback</code> as the callback URL.
        </p>
      </section>
    </main>
  );
}

export function LoginView() {
  return (
    <main>
      <section className="center-card">
        <p className="eyebrow">Local coding workspace</p>
        <h1>Work on a GitHub project with Ayati</h1>
        <p className="muted">
          Choose a repository and branch, initialize one persistent sandbox, then discuss and
          implement the work.
        </p>
        <a className="primary button" href="/auth/github">
          Continue with GitHub
        </a>
      </section>
    </main>
  );
}
