import { BotIcon, CheckIcon } from "lucide-react";
import { type FC, useState } from "react";
import type { ChatAgent } from "#/api/typesGenerated";
import { ChevronDownIcon } from "#/components/AnimatedIcons/ChevronDown";
import { Button } from "#/components/Button/Button";
import {
	Command,
	CommandEmpty,
	CommandGroup,
	CommandInput,
	CommandItem,
	CommandList,
} from "#/components/Command/Command";
import { ExternalImage } from "#/components/ExternalImage/ExternalImage";
import {
	Popover,
	PopoverContent,
	PopoverTrigger,
} from "#/components/Popover/Popover";
import { cn } from "#/utils/cn";

interface AgentSelectorProps {
	options: readonly ChatAgent[];
	value: string;
	onValueChange: (value: string) => void;
	disabled?: boolean;
	placeholder?: string;
	emptyMessage?: string;
	className?: string;
	dropdownSide?: "top" | "bottom" | "left" | "right";
	dropdownAlign?: "start" | "center" | "end";
}

const AgentIcon: FC<{ icon: string; className?: string }> = ({
	icon,
	className,
}) => {
	if (!icon) {
		return <BotIcon aria-hidden="true" className={cn("shrink-0", className)} />;
	}
	return (
		<ExternalImage
			alt=""
			src={icon}
			className={cn("shrink-0 object-contain", className)}
		/>
	);
};

const getSearchText = (agent: ChatAgent) =>
	[agent.name, agent.slug, agent.description].join(" ").toLowerCase();

/**
 * AgentSelector is the chat-creation counterpart of ModelSelector: a
 * compact combobox in the chat input area listing the chat agents
 * (builtin, deployment, and organization) the user can create a chat
 * as.
 */
export const AgentSelector: FC<AgentSelectorProps> = ({
	options,
	value,
	onValueChange,
	disabled = false,
	placeholder = "Select agent",
	emptyMessage = "No agents found.",
	className,
	dropdownSide = "bottom",
	dropdownAlign = "start",
}) => {
	const [open, setOpen] = useState(false);
	const [search, setSearch] = useState("");
	const handleOpenChange = (nextOpen: boolean) => {
		if (!nextOpen) {
			setSearch("");
		}
		setOpen(nextOpen);
	};
	const selectedAgent = options.find((option) => option.id === value);
	const isDisabled = disabled || options.length === 0;
	const query = search.trim().toLowerCase();
	const filteredOptions = query
		? options.filter((option) => getSearchText(option).includes(query))
		: options;

	return (
		<Popover open={open} onOpenChange={handleOpenChange}>
			<PopoverTrigger asChild disabled={isDisabled}>
				<Button
					aria-label={selectedAgent ? selectedAgent.name : placeholder}
					aria-expanded={open}
					aria-haspopup="listbox"
					disabled={isDisabled}
					role="combobox"
					type="button"
					variant="subtle"
					className={cn(
						"h-8 min-w-0 shrink justify-start gap-0.5 border-0 bg-transparent px-1 text-xs font-medium shadow-none transition-colors hover:bg-transparent hover:text-content-primary focus:ring-0 focus-visible:ring-2 focus-visible:ring-content-link md:w-auto md:shrink-0 md:gap-1.5 [&>svg]:shrink-0 [&>svg]:transition-colors [&>svg]:hover:text-content-primary",
						className,
					)}
				>
					<AgentIcon
						icon={selectedAgent?.icon ?? ""}
						className="size-icon-sm"
					/>
					<span className="truncate">
						{selectedAgent ? selectedAgent.name : placeholder}
					</span>
					<ChevronDownIcon open={open} className="size-icon-sm" />
				</Button>
			</PopoverTrigger>
			<PopoverContent
				side={dropdownSide}
				align={dropdownAlign}
				className="w-72 overflow-hidden border-border-default p-0"
			>
				<Command
					shouldFilter={false}
					className="[&_[cmdk-input-wrapper]]:border-0 [&_[cmdk-input-wrapper]]:border-border-default [&_[cmdk-input-wrapper]]:border-b [&_[cmdk-input-wrapper]]:border-solid [&_[cmdk-input-wrapper]]:px-3 [&_[cmdk-input-wrapper]]:py-2 [&_[cmdk-input-wrapper]>svg]:size-3.5"
				>
					<CommandInput
						value={search}
						onValueChange={setSearch}
						placeholder="Search..."
						aria-label="Search agents"
						className="h-auto py-0 text-xs font-normal leading-[18px] text-content-primary placeholder:text-content-disabled"
					/>
					<CommandList role="listbox" className="max-h-80 border-t-0">
						<CommandEmpty className="py-3 text-xs font-normal leading-[18px] text-content-secondary">
							{emptyMessage}
						</CommandEmpty>
						<CommandGroup className="p-1">
							{filteredOptions.map((option) => (
								<CommandItem
									key={option.id}
									value={option.id}
									onSelect={() => {
										onValueChange(option.id);
										handleOpenChange(false);
									}}
									className={cn(
										"gap-2 px-2 py-1 font-medium text-content-secondary data-[selected=true]:bg-surface-tertiary",
										option.id === value && "bg-surface-secondary",
									)}
								>
									<AgentIcon icon={option.icon} className="size-4" />
									<span className="flex min-w-0 flex-col">
										<span className="truncate text-left text-xs font-medium leading-[18px] text-content-primary">
											{option.name}
										</span>
										{option.description && (
											<span className="truncate text-left text-2xs font-normal leading-4 text-content-secondary">
												{option.description}
											</span>
										)}
									</span>
									<CheckIcon
										className={cn(
											"ml-auto size-4 shrink-0",
											option.id !== value && "opacity-0",
										)}
									/>
								</CommandItem>
							))}
						</CommandGroup>
					</CommandList>
				</Command>
			</PopoverContent>
		</Popover>
	);
};
