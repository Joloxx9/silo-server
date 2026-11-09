import { useTrimmedLogo } from "@/hooks/useTrimmedLogo";
import { cn } from "@/lib/utils";

type TitleLogoProps = {
  src: string;
  alt: string;
  className?: string;
  onError?: () => void;
};

/**
 * A title's logo artwork with its transparent padding trimmed, so the size
 * and alignment classes apply to the visible logo rather than to the
 * provider's padded canvas.
 */
export default function TitleLogo({ src, alt, className, onError }: TitleLogoProps) {
  const logo = useTrimmedLogo(src);
  return (
    <img
      src={logo.src}
      alt={alt}
      decoding="async"
      onLoad={logo.onLoad}
      onError={onError}
      className={cn(className, !logo.ready && "opacity-0")}
    />
  );
}
