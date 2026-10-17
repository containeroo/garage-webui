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
  title: string;
  onClose: () => void;
  children: ReactNode;
  className?: string;
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
