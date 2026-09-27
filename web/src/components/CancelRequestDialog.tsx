import { ConfirmDialog } from "@/components/ConfirmDialog";

/** Confirms withdrawing the viewer's own request while it is still pending. */
export function CancelRequestDialog({
  title,
  open,
  onOpenChange,
  onConfirm,
  isPending,
}: {
  /** The requested title, named in the confirmation. */
  title: string;
  open: boolean;
  onOpenChange: (open: boolean) => void;
  onConfirm: () => void;
  isPending?: boolean;
}) {
  return (
    <ConfirmDialog
      open={open}
      onOpenChange={onOpenChange}
      title="Cancel this request?"
      description={`Your request for "${title}" will be withdrawn before an admin reviews it.`}
      confirmLabel="Cancel request"
      cancelLabel="Keep request"
      variant="destructive"
      onConfirm={onConfirm}
      isPending={isPending}
    />
  );
}
