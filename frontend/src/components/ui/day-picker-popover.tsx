import type { ReactElement } from "react";
import { useMemo, useState } from "react";

import { parseDayString, toDayString } from "../../lib/day";
import { cn } from "../../lib/utils";
import { Calendar } from "./calendar";
import { Popover, PopoverContent, PopoverTrigger } from "./popover";

type PopoverAlign = "start" | "center" | "end";

interface DayPickerPopoverProps {
  value: string;
  onChange: (day: string) => void;
  trigger: ReactElement;
  open?: boolean;
  onOpenChange?: (open: boolean) => void;
  align?: PopoverAlign;
  sideOffset?: number;
  contentClassName?: string;
}

export function DayPickerPopover({
  value,
  onChange,
  trigger,
  open: controlledOpen,
  onOpenChange,
  align = "start",
  sideOffset = 8,
  contentClassName,
}: DayPickerPopoverProps): JSX.Element {
  const [uncontrolledOpen, setUncontrolledOpen] = useState(false);
  const open = controlledOpen ?? uncontrolledOpen;
  const setOpen = onOpenChange ?? setUncontrolledOpen;
  const selected = useMemo(() => parseDayString(value), [value]);

  return (
    <Popover open={open} onOpenChange={setOpen}>
      <PopoverTrigger asChild>{trigger}</PopoverTrigger>
      <PopoverContent
        className={cn("day-popover", contentClassName)}
        align={align}
        sideOffset={sideOffset}
      >
        <Calendar
          key={value || "no-day"}
          mode="single"
          selected={selected}
          defaultMonth={selected}
          onSelect={(next) => {
            if (!next) {
              return;
            }
            onChange(toDayString(next));
            setOpen(false);
          }}
          initialFocus
        />
      </PopoverContent>
    </Popover>
  );
}
