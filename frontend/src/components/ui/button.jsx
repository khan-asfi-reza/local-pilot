import { cva } from 'class-variance-authority';
import { cn } from '../../lib/utils';

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-2 whitespace-nowrap rounded-lg text-sm font-medium transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-emerald-500/50 disabled:pointer-events-none disabled:opacity-40',
  {
    variants: {
      variant: {
        default: 'bg-emerald-600 text-white hover:bg-emerald-500',
        ghost: 'text-zinc-300 hover:bg-zinc-800 hover:text-white',
        outline: 'border border-zinc-700/80 bg-transparent text-zinc-200 hover:bg-zinc-800/60',
      },
      size: {
        default: 'h-9 px-4',
        sm: 'h-8 px-3 text-[13px]',
        icon: 'h-9 w-9 rounded-full p-0',
      },
    },
    defaultVariants: { variant: 'default', size: 'default' },
  },
);

export function Button({ className, variant, size, ...props }) {
  return <button className={cn(buttonVariants({ variant, size }), className)} {...props} />;
}
