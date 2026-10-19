import {
  createContext,
  useContext,
  useRef,
  useState,
  type ReactNode,
} from "react";
import { Button } from "./button";
import { DialogPanel } from "./dialog";

type Question = {
  message: string;
  initial?: string;
  resolve: (value: string | boolean | null) => void;
  returnFocus: HTMLElement | null;
};
type Ask = {
  confirm: (message: string) => Promise<boolean>;
  prompt: (message: string, initial?: string) => Promise<string | null>;
};
const QuestionContext = createContext<Ask | null>(null);
export function useQuestions(): Ask {
  const context = useContext(QuestionContext);
  if (!context) throw new Error("question provider missing");
  return context;
}
export function QuestionProvider({ children }: { children: ReactNode }) {
  const [question, setQuestion] = useState<Question>();
  const [value, setValue] = useState("");
  const pending = useRef(false);
  function ask(
    message: string,
    initial?: string,
  ): Promise<string | boolean | null> {
    if (pending.current)
      return Promise.resolve(initial === undefined ? false : null);
    pending.current = true;
    setValue(initial ?? "");
    return new Promise((resolve) =>
      setQuestion({
        message,
        initial,
        resolve,
        returnFocus:
          document.activeElement instanceof HTMLElement
            ? document.activeElement
            : null,
      }),
    );
  }
  function finish(result: string | boolean | null) {
    if (!question) return;
    pending.current = false;
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
