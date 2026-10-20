import * as Dialog from "@radix-ui/react-dialog";
import { X } from "lucide-react";
import type { ReactNode } from "react";

// DialogPanel provides the accessible dialog pattern used by the console.
export function DialogPanel({
  title,
  onClose,
  children,
  className,
  returnFocus,
}: {
  /** Human-readable theme or dialog title. */
  title: string;
  /** Closes the dialog or editor owned by the parent view. */
  onClose: () => void;
  /** Content rendered inside the provider or shared UI container. */
  children: ReactNode;
  /** Optional CSS classes supplied by the containing view. */
  className?: string;
  /** Element to refocus when a nested dialog closes. */
  returnFocus?: HTMLElement | null;
}) {
  return (
    <Dialog.Root
      open
      onOpenChange={(open) => {
        if (!open) onClose();
      }}
    >
      <Dialog.Portal>
        <Dialog.Overlay className="modal-overlay" />
        <Dialog.Content
          className={["modal", className].filter(Boolean).join(" ")}
          aria-describedby={undefined}
          onCloseAutoFocus={
            returnFocus
              ? (event) => {
                  event.preventDefault();
                  returnFocus.focus();
                }
              : undefined
          }
        >
          <div className="modal-head">
            <Dialog.Title asChild>
              <h2>{title}</h2>
            </Dialog.Title>
            <Dialog.Close asChild>
              <button className="icon-button" aria-label="Close dialog">
                <X size={19} />
              </button>
            </Dialog.Close>
          </div>
          {children}
        </Dialog.Content>
      </Dialog.Portal>
    </Dialog.Root>
  );
}
