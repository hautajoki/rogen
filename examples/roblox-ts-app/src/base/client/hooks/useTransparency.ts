// import { atom } from "@rbxts/charm";
// import { useAtom } from "@rbxts/react-charm";
// import { RunService, UserInputService } from "@rbxts/services";

// const preferred_input_atom = atom(UserInputService.PreferredInput);
// RunService.RenderStepped.Connect(() => {
// 	preferred_input_atom(UserInputService.PreferredInput);
// });

// export function usePreferredInput() {
// 	return useAtom(preferred_input_atom);
// }

// export function useIsPreferredController() {
// 	const preferred_input = usePreferredInput();
// 	return preferred_input === Enum.PreferredInput.Gamepad;
// }

// export function useIsPreferredKeyboardAndMouse() {
// 	const preferred_input = usePreferredInput();
// 	return preferred_input === Enum.PreferredInput.KeyboardAndMouse;
// }

// export function useIsPreferredTouch() {
// 	const preferred_input = usePreferredInput();
// 	return preferred_input === Enum.PreferredInput.Touch;
// }
