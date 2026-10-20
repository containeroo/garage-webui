import {
  createContext,
  useEffect,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Button } from "./button";
import { DialogPanel } from "./dialog";

/** An outstanding confirmation or prompt and its result continuation. */
type Question = {
  /** Explanatory messages returned by Garage or shown in a prompt. */
  message: string;
  /** Initial text value; absent indicates a boolean confirmation. */
  initial?: string;
  /** Settles the outstanding prompt with the user's response. */
  resolve: (value: string | boolean | null) => void;
  /** Element to refocus when a nested dialog closes. */
  returnFocus: HTMLElement | null;
};
/** The asynchronous confirmation and text-prompt operations exposed to views. */
type Ask = {
  /** Requests an explicit yes-or-no decision from the user. */
  confirm: (message: string) => Promise<boolean>;
  /** Requests a text value and returns null when dismissed. */
  prompt: (message: string, initial?: string) => Promise<string | null>;
};
const QuestionContext = createContext<Ask | null>(null);
/** useQuestions accesses asynchronous prompts within a QuestionProvider. */
export function useQuestions(): Ask {
  const context = useContext(QuestionContext);
  if (!context) throw new Error("question provider missing");
  return context;
}
/** QuestionProvider manages one accessible prompt and settles it on dismissal or unmount. */
export function QuestionProvider({
  children,
}: {
  /** Content rendered inside the provider or shared UI container. */
  children: ReactNode;
}) {
  const [question, setQuestion] = useState<Question>();
  const [value, setValue] = useState("");
  const pending = useRef<Question | null>(null);
  useEffect(
    () => () => {
      const unresolved = pending.current;
      pending.current = null;
      unresolved?.resolve(unresolved.initial === undefined ? false : null);
    },
    [],
  );
  /** ask opens one prompt and declines overlapping requests without replacing the active question. */
  function ask(
    message: string,
    initial?: string,
  ): Promise<string | boolean | null> {
    if (pending.current)
      return Promise.resolve(initial === undefined ? false : null);
    setValue(initial ?? "");
    return new Promise((resolve) => {
      const next: Question = {
        message,
        initial,
        resolve,
        returnFocus:
          document.activeElement instanceof HTMLElement
            ? document.activeElement
            : null,
      };
      pending.current = next;
      setQuestion(next);
    });
  }
  /** finish resolves the pending prompt and releases its dialog state. */
  function finish(result: string | boolean | null) {
    if (!question) return;
    pending.current = null;
    question.resolve(result);
    setQuestion(undefined);
  }
  return (
    <QuestionContext.Provider
      value={{
        confirm: async (message) => (await ask(message)) === true,
        prompt: async (message, initial = "") => {
          const result = await ask(message, initial);
          return typeof result === "string" ? result : null;
        },
      }}
    >
      {children}
      {question && (
        <DialogPanel
          className="question-dialog"
          title={
            question.initial === undefined ? "Please confirm" : "Enter a value"
          }
          onClose={() => finish(question.initial === undefined ? false : null)}
          returnFocus={question.returnFocus}
        >
          <form
            className="question-body"
            onSubmit={(event) => {
              event.preventDefault();
              finish(question.initial === undefined ? true : value);
            }}
          >
            <p>{question.message}</p>
            {question.initial !== undefined && (
              <input
                aria-label={question.message}
                autoFocus
                value={value}
                onChange={(event) => setValue(event.target.value)}
              />
            )}
            <div className="question-actions">
              <Button
                type="button"
                variant="outline"
                onClick={() =>
                  finish(question.initial === undefined ? false : null)
                }
              >
                Cancel
              </Button>
              <Button type="submit">Confirm</Button>
            </div>
          </form>
        </DialogPanel>
      )}
    </QuestionContext.Provider>
  );
}
