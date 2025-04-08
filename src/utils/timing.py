import functools
import time
from typing import Any, Callable, Tuple, TypeVar, cast

F = TypeVar("F", bound=Callable[..., Any])

def retry(
    retries: int = 3,
    delay: float = 0.5,
    backoff: bool = False,
    backoff_ratio: float = 2,
    exceptions: Tuple[type[BaseException], ...] = (Exception,),
    logger_func: Callable[[str], None] = print,
) -> Callable[[F], F]:
    """
    A simple retry decorator with delay and optional logger.
    """

    def decorator(func: F) -> F:
        @functools.wraps(func)
        def wrapper(*args: Any, **kwargs: Any) -> Any:
            current_delay = delay
            for attempt in range(1, retries + 1):
                try:
                    return func(*args, **kwargs)
                except exceptions as e:
                    if attempt == retries:
                        raise
                    logger_func(f"[retry] Attempt {attempt} failed: {e}. Retrying in {current_delay}s...")
                    time.sleep(current_delay)
                    if backoff:
                        current_delay *= backoff_ratio

        return cast(F, wrapper)

    return decorator