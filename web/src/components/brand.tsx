// Brand uses the bundled logo on both the sign-in page and the sidebar.
export function Brand({
  className = "",
}: {
  /** Optional CSS classes supplied by the containing view. */
  className?: string;
}) {
  const logo = `url("${new URL("logo.svg", document.baseURI).toString()}")`;
  return (
    <div className={`brand ${className}`}>
      <div className="brand-icon" aria-hidden="true">
        <div
          className="brand-mark"
          style={{ maskImage: logo, WebkitMaskImage: logo }}
        />
      </div>
      <div>
        <strong>garage</strong>
        <span>STORAGE CONSOLE</span>
      </div>
    </div>
  );
}
