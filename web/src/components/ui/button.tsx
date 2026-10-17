import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "../../lib/utils";

// buttonVariants defines the shared console button styles.
const buttonVariants = cva(
  "inline-flex min-h-9 items-center justify-center gap-2 rounded-md text-sm font-medium transition-colors focus-visible:outline-2 focus-visible:outline-offset-2 focus-visible:outline-indigo-500 disabled:pointer-events-none disabled:opacity-50",
  {
    variants: {
      variant: {
        default: "bg-zinc-900 px-4 py-2 text-white hover:bg-zinc-700",
        outline:
          "border border-zinc-200 bg-white px-4 py-2 text-zinc-900 hover:bg-zinc-100",
      },
    },
    defaultVariants: { variant: "default" },
  },
);

// Button renders a shared action or styles its child as an action.
export function Button({
  className,
  variant,
  asChild = false,
  ...props
}: ComponentProps<"button"> &
  VariantProps<typeof buttonVariants> & { asChild?: boolean }) {
  const Component = asChild ? Slot : "button";
  return (
    <Component
      className={cn(buttonVariants({ variant }), className)}
      {...props}
    />
  );
}
