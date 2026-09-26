import { Ban, Check, RefreshCw, X } from "lucide-react";
import type { LucideIcon } from "lucide-react";
import type { MediaRequest } from "@/api/types";
import { Button } from "@/components/ui/button";
import { requestQueueActions, type RequestQueueAction } from "./requestQueueModel";

/** Runs a queue action; decline and cancel ask for an optional reason first. */
export interface RequestQueueActionHandlers {
  run: (action: RequestQueueAction, request: MediaRequest) => void;
  /** An action for this request is on its way to the server. */
  isBusy: (id: string) => boolean;
  /** Every action waits, as while a bulk action runs. */
  locked: boolean;
}

const ACTIONS: Record<
  RequestQueueAction,
  { label: string; icon: LucideIcon; variant: "default" | "outline" }
> = {
  approve: { label: "Approve", icon: Check, variant: "default" },
  decline: { label: "Decline", icon: X, variant: "outline" },
  retry: { label: "Retry", icon: RefreshCw, variant: "default" },
  cancel: { label: "Cancel request", icon: Ban, variant: "outline" },
};

/** The actions a request allows in the queue; none for one that is done. */
export function RequestActionButtons({
  request,
  handlers,
}: {
  request: MediaRequest;
  handlers: RequestQueueActionHandlers;
}) {
  const actions = requestQueueActions(request);
  if (actions.length === 0) return null;
  const disabled = handlers.locked || handlers.isBusy(request.id);
  return (
    <div className="flex flex-wrap gap-2">
      {actions.map((action) => {
        const { label, icon: Icon, variant } = ACTIONS[action];
        return (
          <Button
            key={action}
            size="sm"
            variant={variant}
            disabled={disabled}
            aria-label={`${label}: ${request.title}`}
            onClick={() => handlers.run(action, request)}
          >
            <Icon aria-hidden="true" />
            {label}
          </Button>
        );
      })}
    </div>
  );
}
