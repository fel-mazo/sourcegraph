import DeleteIcon from '@sourcegraph/icons/lib/Delete'
import WarningIcon from '@sourcegraph/icons/lib/Warning'
import { upperFirst } from 'lodash'
import * as React from 'react'
import { Subject, Subscription } from 'rxjs'
import { catchError, map, mapTo, startWith, switchMap, tap } from 'rxjs/operators'
import * as GQL from '../backend/graphqlschema'
import { asError, ErrorLike, isErrorLike } from '../util/errors'
import { deleteRegistryExtensionWithConfirmation } from './backend'

interface RegistryExtensionDeleteButtonProps {
    extension: Pick<GQL.IRegistryExtension, 'id'>

    compact?: boolean

    className?: string
    disabled?: boolean

    /** Called when the extension is deleted. */
    onDidUpdate: () => void
}

interface RegistryExtensionDeleteButtonState {
    /** Undefined means in progress, null means done or not started. */
    deletionOrError?: null | ErrorLike
}

/** A button that deletes an extension from the registry. */
export class RegistryExtensionDeleteButton extends React.PureComponent<
    RegistryExtensionDeleteButtonProps,
    RegistryExtensionDeleteButtonState
> {
    public state: RegistryExtensionDeleteButtonState = {
        deletionOrError: null,
    }

    private deletes = new Subject<void>()
    private subscriptions = new Subscription()

    public componentDidMount(): void {
        this.subscriptions.add(
            this.deletes
                .pipe(
                    switchMap(args =>
                        deleteRegistryExtensionWithConfirmation(this.props.extension.id).pipe(
                            mapTo(null),
                            catchError(error => [asError(error)]),
                            map(c => ({ deletionOrError: c })),
                            tap(() => {
                                if (this.props.onDidUpdate) {
                                    this.props.onDidUpdate()
                                }
                            }),
                            startWith<Pick<RegistryExtensionDeleteButtonState, 'deletionOrError'>>({
                                deletionOrError: undefined,
                            })
                        )
                    )
                )
                .subscribe(stateUpdate => this.setState(stateUpdate), error => console.error(error))
        )
    }

    public componentWillUnmount(): void {
        this.subscriptions.unsubscribe()
    }

    public render(): JSX.Element | null {
        return (
            <div className="btn-group" role="group">
                <button
                    className="btn btn-outline-danger"
                    onClick={this.deleteExtension}
                    disabled={this.props.disabled || this.state.deletionOrError === undefined}
                    title={this.props.compact ? 'Delete extension' : ''}
                >
                    <DeleteIcon className="icon-inline" /> {!this.props.compact && 'Delete extension'}
                </button>
                {isErrorLike(this.state.deletionOrError) && (
                    <button
                        disabled={true}
                        className="btn btn-danger"
                        title={upperFirst(this.state.deletionOrError.message)}
                    >
                        <WarningIcon className="icon-inline" />
                    </button>
                )}
            </div>
        )
    }

    private deleteExtension = () => this.deletes.next()
}
