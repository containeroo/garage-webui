import { Slot } from "@radix-ui/react-slot";
import { cva, type VariantProps } from "class-variance-authority";
import type { ComponentProps } from "react";
import { cn } from "../../lib/utils";

// buttonVariants defines the shared console button styles.
const buttonVariants = cva(
  "inline-flex min-h-9 items-center justify-center gap-2 text-sm font-medium disabled:pointer-events-none",
  {
    variants: {
      variant: {
        default: "primary",
        outline: "secondary",
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
  VariantProps<typeof buttonVariants> & {
    /** Whether the child element receives the shared button appearance. */
    asChild?: boolean;
  }) {
  const Component = asChild ? Slot : "button";
  return (
    <Component
      className={cn(buttonVariants({ variant }), className)}
      {...props}
    />
  );
}
